// Package logging builds the daemon's slog.Logger: a rotating log file (via
// lumberjack), a stdout mirror for console builds, an in-memory Ring of recent
// formatted lines for GET /api/logs, and a bridge that streams records to the
// events hub so the in-app Logs view updates live. main.go wires these together;
// this package keeps the assembly out of it.
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/modbender/ssanime-gui/internal/events"
)

// stdout is the console mirror. In -H=windowsgui builds it is a null handle, so
// writes are discarded harmlessly; in console builds it receives every line.
func stdout() io.Writer { return os.Stdout }

// FileName is the active log file's name under the data dir. lumberjack keeps the
// active file at this exact path and rotates older data into siblings, so anything
// tailing this path (e.g. GET /api/logs) keeps seeing the live tail.
const FileName = "ssanime.log"

// ringCapacity is how many recent formatted lines the in-memory Ring keeps for
// GET /api/logs to serve the Logs page's historic section.
const ringCapacity = 500

// Rotation policy: 10 MB per file, keep 30 rotated backups, drop anything older
// than 30 days, gzip the rotated files.
const (
	maxSizeMB  = 10
	maxBackups = 30
	maxAgeDays = 30
)

// Build assembles the daemon logger. It returns the logger, a *HubBridge whose
// hub-side stays inert until Attach is called, a *Ring holding the last
// ringCapacity formatted lines (for GET /api/logs), and an io.Closer that flushes
// and closes the rotating file sink (the caller defers it).
//
// The logger fans out two ways: a text handler writing to file+stdout+ring, and
// the bridge broadcasting Info+ records to the events hub. Both honour LevelInfo.
// The ring is a third sink on the text handler's MultiWriter, so it captures
// exactly the bytes written to the file — historic (/api/logs) and on-disk logs
// never diverge.
func Build(dataDir string) (*slog.Logger, *HubBridge, *Ring, io.Closer) {
	rot := &lumberjack.Logger{
		Filename:   filepath.Join(dataDir, FileName),
		MaxSize:    maxSizeMB,
		MaxBackups: maxBackups,
		MaxAge:     maxAgeDays,
		Compress:   true,
	}
	ring := newRing(ringCapacity)
	w := io.MultiWriter(rot, stdout(), ring)
	text := slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo})

	bridge := &HubBridge{}
	handler := &fanout{text: text, bridge: bridge}
	return slog.New(handler), bridge, ring, rot
}

// fanout dispatches every record to the text handler (file+stdout) and the bridge
// (events hub). It is a thin slog.Handler that forwards Enabled/WithAttrs/WithGroup
// to the text handler and lets the bridge make its own level/attr decisions.
type fanout struct {
	text   slog.Handler
	bridge *HubBridge
}

func (f *fanout) Enabled(ctx context.Context, l slog.Level) bool { return f.text.Enabled(ctx, l) }

func (f *fanout) Handle(ctx context.Context, r slog.Record) error {
	f.bridge.handle(r)
	return f.text.Handle(ctx, r)
}

func (f *fanout) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &fanout{text: f.text.WithAttrs(attrs), bridge: f.bridge}
}

func (f *fanout) WithGroup(name string) slog.Handler {
	return &fanout{text: f.text.WithGroup(name), bridge: f.bridge}
}

// HubBridge streams slog records to the events hub as TypeLog events. It is built
// before the hub exists (the hub is created inside startDaemon), so the hub pointer
// is nil until Attach wires it. Before Attach, handle is a no-op on the hub side;
// records still reach the file via the text handler.
type HubBridge struct {
	mu  sync.RWMutex
	hub *events.Hub
}

// Attach wires the live hub. Safe to call once, after hub.Start().
func (b *HubBridge) Attach(hub *events.Hub) {
	b.mu.Lock()
	b.hub = hub
	b.mu.Unlock()
}

// logPayload is the FROZEN wire shape the frontend SSE client expects for "log"
// events: ts is unix SECONDS (the frontend multiplies by 1000).
type logPayload struct {
	Level   string `json:"level"`
	Message string `json:"message"`
	TS      int64  `json:"ts"`
}

// hubInternalPrefix marks records the events hub itself emits (e.g. dropping a
// slow client). Recursion guard: the hub's only on-broadcast log is drop()'s
// "events: dropped slow SSE client" Warn. Re-broadcasting it would loop
// (Broadcast -> drop -> Warn -> handle -> Broadcast). We skip records with this
// prefix so the bridge never feeds the hub a record the hub produced.
const hubInternalPrefix = "events:"

// handle broadcasts one record to the hub. It is safe for concurrent use and is a
// no-op before Attach or below Info. Attrs are appended to the message as
// " key=val" pairs so contextual fields aren't lost in the UI.
func (b *HubBridge) handle(r slog.Record) {
	if r.Level < slog.LevelInfo {
		return
	}
	if strings.HasPrefix(r.Message, hubInternalPrefix) {
		return
	}
	b.mu.RLock()
	hub := b.hub
	b.mu.RUnlock()
	if hub == nil {
		return
	}

	var sb strings.Builder
	sb.WriteString(r.Message)
	r.Attrs(func(a slog.Attr) bool {
		sb.WriteByte(' ')
		sb.WriteString(a.Key)
		sb.WriteByte('=')
		sb.WriteString(a.Value.String())
		return true
	})

	hub.Broadcast(events.TypeLog, logPayload{
		Level:   levelString(r.Level),
		Message: sb.String(),
		TS:      r.Time.Unix(),
	})
}

// Ring is a bounded, thread-safe buffer of the most recent formatted log lines.
// It implements io.Writer so it can sit on the text handler's MultiWriter: each
// Write splits incoming bytes on newlines, completing whole lines (a final
// partial line is held until the newline that finishes it arrives). It serves
// GET /api/logs via Lines.
//
// slog's TextHandler emits exactly one Write per record terminated by '\n', so in
// practice each Write is one complete line; the split/carry logic is defensive
// against partial or batched writes and never feeds the ring a half-line.
type Ring struct {
	mu      sync.Mutex
	buf     []string
	cap     int
	head    int
	size    int
	partial []byte // bytes since the last newline, not yet a complete line
}

func newRing(n int) *Ring {
	return &Ring{buf: make([]string, n), cap: n}
}

// Write splits p on '\n' and stores each completed line, evicting the oldest when
// full. A trailing fragment (no terminating newline) is carried into the next
// Write. It always reports len(p) consumed and never errors, so it never breaks
// the surrounding io.MultiWriter (a non-nil error would abort writes to later
// sinks). This is the hot path: append to a small slice under a plain Mutex.
func (r *Ring) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, b := range p {
		if b == '\n' {
			r.push(string(r.partial))
			r.partial = r.partial[:0]
			continue
		}
		r.partial = append(r.partial, b)
	}
	return len(p), nil
}

// push stores one completed line into the circular buffer. Caller holds r.mu.
func (r *Ring) push(line string) {
	r.buf[r.head] = line
	r.head = (r.head + 1) % r.cap
	if r.size < r.cap {
		r.size++
	}
}

// Lines returns up to limit recent lines (newest last). limit<=0 means all. Safe
// for concurrent use with Write.
func (r *Ring) Lines(limit int) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := r.size
	if limit > 0 && limit < n {
		n = limit
	}
	out := make([]string, n)
	start := (r.head - n + r.cap) % r.cap
	for i := 0; i < n; i++ {
		out[i] = r.buf[(start+i)%r.cap]
	}
	return out
}

// levelString lowercases the slog level to the frontend's contract values.
func levelString(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "error"
	case l >= slog.LevelWarn:
		return "warn"
	case l >= slog.LevelInfo:
		return "info"
	default:
		return "debug"
	}
}
