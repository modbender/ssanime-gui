package extension

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// callTimeout bounds a single JS method call so a hung extension can't stall
// the daemon.
const callTimeout = 15 * time.Second

// stripExportDefault rewrites "export default <expr>" to
// "var __ssExt = (<expr>)" so goja can execute it as a plain script.
// This handles the single pattern all known JS extensions use
// ("export default new class ...").
//
// The captured expression is trimmed of trailing semicolons/whitespace before
// being wrapped: the exten.pages.dev extensions end their default export with
// "};" and a stray ";" inside the wrapping parens is a syntax error.
func stripExportDefault(src string) string {
	const marker = "export default"
	idx := strings.Index(src, marker)
	if idx == -1 {
		return src
	}
	expr := strings.TrimSpace(src[idx+len(marker):])
	expr = strings.TrimRight(expr, "; \t\r\n")
	var b bytes.Buffer
	b.WriteString(src[:idx])
	b.WriteString("var __ssExt = (")
	b.WriteString(expr)
	b.WriteString(")")
	return b.String()
}

// VM wraps a per-call goja runtime + event loop for a single extension.
// Each call to CallMethod spins a fresh event loop, injects host bindings,
// executes the compiled program, invokes the JS method, and waits for any
// returned Promise to settle — then stops the loop.
type VM struct {
	program *goja.Program // compiled once, reused per-call
	http    *http.Client  // host HTTP client backing the JS fetch() shim
	logger  *slog.Logger
	extID   string
}

// NewVM compiles the JS payload and returns a VM.
func NewVM(extID, payload string, httpClient *http.Client, logger *slog.Logger) (*VM, error) {
	src := stripExportDefault(payload)
	prog, err := goja.Compile(extID+".js", src, false)
	if err != nil {
		return nil, fmt.Errorf("extension %s: compile: %w", extID, err)
	}
	return &VM{
		program: prog,
		http:    httpClient,
		logger:  logger,
		extID:   extID,
	}, nil
}

// CallMethod invokes methodName on the extension object, resolves any returned
// Promise, and returns the exported Go value. It is safe to call concurrently —
// each call builds its own event loop + runtime.
func (v *VM) CallMethod(ctx context.Context, methodName string, args ...interface{}) (interface{}, error) {
	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	type result struct {
		val interface{}
		err error
	}
	ch := make(chan result, 1)

	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Terminate()

	// Capture the loop's runtime so the timeout path can preempt a runaway
	// synchronous loop (goja is not preemptible; Terminate alone can't stop a
	// `while(true){}` that never yields back to the event loop).
	var rtMu sync.Mutex
	var activeRT *goja.Runtime

	loop.RunOnLoop(func(rt *goja.Runtime) {
		rtMu.Lock()
		activeRT = rt
		rtMu.Unlock()
		// Inject host bindings before running the program.
		v.bindConsole(rt)
		v.bindFetch(loop, rt)
		v.bindSetTimeout(rt)
		v.bindGlobals(rt)

		// Run the compiled program; this defines __ssExt.
		if _, err := rt.RunProgram(v.program); err != nil {
			ch <- result{err: fmt.Errorf("extension %s: run: %w", v.extID, err)}
			return
		}

		extVal := rt.Get("__ssExt")
		if extVal == nil || goja.IsNull(extVal) || goja.IsUndefined(extVal) {
			ch <- result{err: fmt.Errorf("extension %s: __ssExt undefined after run", v.extID)}
			return
		}
		obj := extVal.ToObject(rt)

		fn, ok := goja.AssertFunction(obj.Get(methodName))
		if !ok {
			ch <- result{err: fmt.Errorf("extension %s: method %q not found or not a function", v.extID, methodName)}
			return
		}

		gojaArgs := make([]goja.Value, len(args))
		for i, a := range args {
			gojaArgs[i] = nativeJSValue(rt, a)
		}

		// Hayase passes its host fetch in the options object (extensions
		// destructure `fetch` from the first arg and call it). Inject the global
		// fetch onto the first argument when it's an object and doesn't already
		// carry one, so `single({fetch}, opts)` works without each extension
		// reaching for a global.
		if len(gojaArgs) > 0 {
			if argObj, ok := gojaArgs[0].(*goja.Object); ok {
				if f := argObj.Get("fetch"); f == nil || goja.IsUndefined(f) || goja.IsNull(f) {
					_ = argObj.Set("fetch", rt.Get("fetch"))
				}
			}
		}

		callResult, err := fn(obj, gojaArgs...)
		if err != nil {
			ch <- result{err: fmt.Errorf("extension %s: %s: %w", v.extID, methodName, err)}
			return
		}

		// If the result is a Promise, install a .then/.catch handler that
		// sends the settled value back on ch via the event loop.
		if promise, ok := callResult.Export().(*goja.Promise); ok {
			thenFn := rt.ToValue(func(call goja.FunctionCall) goja.Value {
				exported := call.Argument(0).Export()
				ch <- result{val: exported}
				return goja.Undefined()
			})
			catchFn := rt.ToValue(func(call goja.FunctionCall) goja.Value {
				msg := call.Argument(0).String()
				ch <- result{err: fmt.Errorf("promise rejected: %s", msg)}
				return goja.Undefined()
			})
			// rt.ToValue on a *Promise returns the underlying *goja.Object.
			promObj := rt.ToValue(promise).(*goja.Object)
			thenMethod, _ := goja.AssertFunction(promObj.Get("then"))
			_, _ = thenMethod(promObj, thenFn, catchFn)
		} else {
			// Synchronous result.
			ch <- result{val: callResult.Export()}
		}
	})

	// Wait for the result or context cancellation.
	select {
	case r := <-ch:
		return r.val, r.err
	case <-callCtx.Done():
		// Interrupt any JS executing on the loop goroutine so a malicious or
		// buggy extension stuck in a tight synchronous loop is actually torn
		// down rather than leaking a goroutine that spins a CPU forever.
		rtMu.Lock()
		if activeRT != nil {
			activeRT.Interrupt("extension call timed out")
		}
		rtMu.Unlock()
		loop.Terminate()
		return nil, fmt.Errorf("extension %s: %s: %w", v.extID, methodName, callCtx.Err())
	}
}

// nativeJSValue converts a Go argument into a native JS value by round-tripping
// through the runtime's JSON.parse. Passing a Go map/slice straight to ToValue
// yields host-wrapped objects whose arrays fail Array.isArray and whose elements
// aren't real JS strings, which breaks extensions that branch on those. A
// JSON-parsed value is indistinguishable from one the script built itself.
// Falls back to ToValue for anything that can't be marshalled (e.g. no args).
func nativeJSValue(rt *goja.Runtime, a interface{}) goja.Value {
	data, err := json.Marshal(a)
	if err != nil {
		return rt.ToValue(a)
	}
	jsonObj := rt.Get("JSON").ToObject(rt)
	parse, ok := goja.AssertFunction(jsonObj.Get("parse"))
	if !ok {
		return rt.ToValue(a)
	}
	v, err := parse(goja.Undefined(), rt.ToValue(string(data)))
	if err != nil {
		return rt.ToValue(a)
	}
	return v
}

// bindConsole wires console.log/warn/error to slog.
func (v *VM) bindConsole(rt *goja.Runtime) {
	cons := rt.NewObject()
	for _, level := range []string{"log", "info", "warn", "error", "debug"} {
		lvl := level
		_ = cons.Set(lvl, func(call goja.FunctionCall) goja.Value {
			parts := make([]string, len(call.Arguments))
			for i, a := range call.Arguments {
				parts[i] = fmt.Sprintf("%v", a.Export())
			}
			v.logger.Debug("js.console."+lvl, "ext", v.extID, "msg", strings.Join(parts, " "))
			return goja.Undefined()
		})
	}
	rt.Set("console", cons)
}

// bindFetch injects a fetch() implementation backed by the host HTTP client.
// The goroutine that performs the HTTP call uses loop.RunOnLoop to call
// resolve/reject back on the event loop thread — the only safe way with goja.
func (v *VM) bindFetch(loop *eventloop.EventLoop, rt *goja.Runtime) {
	rt.Set("fetch", func(call goja.FunctionCall) goja.Value {
		urlStr := call.Argument(0).String()

		// Parse optional init object.
		method := http.MethodGet
		var reqBody io.Reader
		headers := make(map[string]string)

		if len(call.Arguments) > 1 {
			init := call.Arguments[1].Export()
			if m, ok := init.(map[string]interface{}); ok {
				if meth, ok := m["method"].(string); ok {
					method = strings.ToUpper(meth)
				}
				if h, ok := m["headers"].(map[string]interface{}); ok {
					for k, hv := range h {
						headers[k] = fmt.Sprintf("%v", hv)
					}
				}
				if b, ok := m["body"].(string); ok {
					reqBody = strings.NewReader(b)
				}
			}
		}

		promise, resolve, reject := rt.NewPromise()

		go func() {
			req, err := http.NewRequest(method, urlStr, reqBody)
			if err != nil {
				loop.RunOnLoop(func(*goja.Runtime) { _ = reject(err.Error()) })
				return
			}
			for k, hv := range headers {
				req.Header.Set(k, hv)
			}

			resp, err := v.http.Do(req)
			if err != nil {
				loop.RunOnLoop(func(*goja.Runtime) { _ = reject(err.Error()) })
				return
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				loop.RunOnLoop(func(*goja.Runtime) { _ = reject(err.Error()) })
				return
			}

			// Capture values for the loop callback.
			statusCode := resp.StatusCode
			bodyStr := string(body)
			bodyBytes := body
			respHeaders := resp.Header

			loop.RunOnLoop(func(rt2 *goja.Runtime) {
				respObj := rt2.NewObject()
				_ = respObj.Set("ok", statusCode >= 200 && statusCode < 300)
				_ = respObj.Set("status", statusCode)
				_ = respObj.Set("statusText", fmt.Sprintf("%d", statusCode))

				_ = respObj.Set("text", func(goja.FunctionCall) goja.Value {
					p2, r2, _ := rt2.NewPromise()
					_ = r2(bodyStr)
					return rt2.ToValue(p2)
				})

				_ = respObj.Set("json", func(goja.FunctionCall) goja.Value {
					p2, r2, rj2 := rt2.NewPromise()
					var parsed interface{}
					if jsonErr := json.Unmarshal(bodyBytes, &parsed); jsonErr != nil {
						_ = rj2(nonJSONError(statusCode, bodyStr))
					} else {
						_ = r2(rt2.ToValue(parsed))
					}
					return rt2.ToValue(p2)
				})

				headersObj := rt2.NewObject()
				_ = headersObj.Set("get", func(call goja.FunctionCall) goja.Value {
					return rt2.ToValue(respHeaders.Get(call.Argument(0).String()))
				})
				_ = respObj.Set("headers", headersObj)

				_ = resolve(rt2.ToValue(respObj))
			})
		}()

		return rt.ToValue(promise)
	})
}

// bindGlobals injects the browser globals Hayase extensions touch at load and
// during search: atob/btoa (base64) and a minimal navigator object. Several
// exten.pages.dev extensions decode their API base with atob(...) at module
// scope and read navigator.onLine before searching; without these the program
// throws ReferenceError before any method runs.
func (v *VM) bindGlobals(rt *goja.Runtime) {
	rt.Set("atob", func(call goja.FunctionCall) goja.Value {
		decoded, err := decodeBase64Lenient(call.Argument(0).String())
		if err != nil {
			// Mirror the browser: invalid input throws (catchable in JS).
			panic(rt.NewTypeError("atob: invalid base64 input"))
		}
		return rt.ToValue(string(decoded))
	})
	rt.Set("btoa", func(call goja.FunctionCall) goja.Value {
		return rt.ToValue(base64.StdEncoding.EncodeToString([]byte(call.Argument(0).String())))
	})

	nav := rt.NewObject()
	_ = nav.Set("onLine", true)
	_ = nav.Set("userAgent", "ssanime")
	rt.Set("navigator", nav)

	// URLSearchParams: goja has no DOM/WHATWG globals. Several extensions build
	// query strings with `new URLSearchParams({...})`, `.append`, and
	// `.toString()` (string-coerced into the request URL). A small JS polyfill
	// covering the surface these extensions use is injected here; a failure to
	// install it is fatal to compatibility, so it panics (caught as a run error).
	if _, err := rt.RunString(urlSearchParamsPolyfill); err != nil {
		panic(rt.ToValue("failed to install URLSearchParams polyfill: " + err.Error()))
	}
}

// urlSearchParamsPolyfill is a minimal WHATWG URLSearchParams implementation
// covering construction from a record/string/iterable, append/set/get/getAll/
// has/delete, and toString() with proper percent-encoding (application/x-www-
// form-urlencoded: space → '+'). It is intentionally small — enough for the
// Hayase extensions that build query strings.
const urlSearchParamsPolyfill = `
(function (global) {
  if (typeof global.URLSearchParams === 'function') return;
  function enc(s) {
    return encodeURIComponent(String(s)).replace(/%20/g, '+');
  }
  function USP(init) {
    this._p = [];
    if (init == null) return;
    if (typeof init === 'string') {
      var s = init.charAt(0) === '?' ? init.slice(1) : init;
      if (s.length) {
        var self = this;
        s.split('&').forEach(function (pair) {
          if (!pair) return;
          var i = pair.indexOf('=');
          var k = i === -1 ? pair : pair.slice(0, i);
          var v = i === -1 ? '' : pair.slice(i + 1);
          self._p.push([decodeURIComponent(k.replace(/\+/g, ' ')), decodeURIComponent(v.replace(/\+/g, ' '))]);
        });
      }
    } else if (Array.isArray(init)) {
      for (var j = 0; j < init.length; j++) this._p.push([String(init[j][0]), String(init[j][1])]);
    } else if (typeof init === 'object') {
      for (var key in init) {
        if (Object.prototype.hasOwnProperty.call(init, key)) this._p.push([key, String(init[key])]);
      }
    }
  }
  USP.prototype.append = function (k, v) { this._p.push([String(k), String(v)]); };
  USP.prototype.set = function (k, v) {
    k = String(k); var found = false; var out = [];
    for (var i = 0; i < this._p.length; i++) {
      if (this._p[i][0] === k) { if (!found) { out.push([k, String(v)]); found = true; } }
      else out.push(this._p[i]);
    }
    if (!found) out.push([k, String(v)]);
    this._p = out;
  };
  USP.prototype.get = function (k) {
    k = String(k);
    for (var i = 0; i < this._p.length; i++) if (this._p[i][0] === k) return this._p[i][1];
    return null;
  };
  USP.prototype.getAll = function (k) {
    k = String(k); var out = [];
    for (var i = 0; i < this._p.length; i++) if (this._p[i][0] === k) out.push(this._p[i][1]);
    return out;
  };
  USP.prototype.has = function (k) { return this.get(String(k)) !== null; };
  USP.prototype['delete'] = function (k) {
    k = String(k); var out = [];
    for (var i = 0; i < this._p.length; i++) if (this._p[i][0] !== k) out.push(this._p[i]);
    this._p = out;
  };
  USP.prototype.forEach = function (cb, thisArg) {
    for (var i = 0; i < this._p.length; i++) cb.call(thisArg, this._p[i][1], this._p[i][0], this);
  };
  USP.prototype.toString = function () {
    var out = [];
    for (var i = 0; i < this._p.length; i++) out.push(enc(this._p[i][0]) + '=' + enc(this._p[i][1]));
    return out.join('&');
  };
  global.URLSearchParams = USP;
})(this);
`

// decodeBase64Lenient decodes base64 the way a browser atob tolerates input:
// strip all whitespace, fix missing padding, and accept both standard and
// URL-safe alphabets (with or without padding). Returns an error only when no
// variant decodes.
func decodeBase64Lenient(s string) ([]byte, error) {
	// Remove all ASCII whitespace (spaces, tabs, newlines) the way atob does.
	s = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r', '\v', '\f':
			return -1
		}
		return r
	}, s)
	if s == "" {
		return []byte{}, nil
	}
	// Try padded variants as-is first.
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return b, nil
		}
	}
	// Try raw (unpadded) variants.
	for _, enc := range []*base64.Encoding{base64.RawStdEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return b, nil
		}
	}
	// Last resort: re-pad to a multiple of 4 and retry the padded alphabets.
	if pad := len(s) % 4; pad != 0 {
		s += strings.Repeat("=", 4-pad)
		for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding} {
			if b, err := enc.DecodeString(s); err == nil {
				return b, nil
			}
		}
	}
	return nil, fmt.Errorf("invalid base64")
}

// nonJSONError builds the actionable message surfaced when fetch().json() is
// called on a non-JSON body (the common "dead proxy returns HTML" case),
// replacing the cryptic raw "invalid character 'T'" decode error.
func nonJSONError(status int, body string) string {
	snippet := strings.TrimSpace(body)
	const limit = 80
	if len(snippet) > limit {
		snippet = snippet[:limit]
	}
	return fmt.Sprintf("non-JSON response (HTTP %d): %s", status, snippet)
}

// bindSetTimeout provides a synchronous no-op setTimeout. Extensions that use
// it (e.g. for delays) will fire immediately; actual timer semantics require
// a full event loop integration that is out of scope for provider calls.
func (v *VM) bindSetTimeout(rt *goja.Runtime) {
	rt.Set("setTimeout", func(call goja.FunctionCall) goja.Value {
		if fn, ok := goja.AssertFunction(call.Argument(0)); ok {
			_, _ = fn(goja.Undefined())
		}
		return rt.ToValue(0)
	})
	rt.Set("clearTimeout", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
}
