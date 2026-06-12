package extension

import (
	"context"
	"net/http"
	"testing"
)

// TestNativeArrayMarshaling guards the input-marshaling contract: a Go []string
// passed as a method argument must arrive in JS as a genuine Array of genuine
// strings — Array.isArray(titles) true and titles[0] a usable string with
// string methods. A naive rt.ToValue([]string) yields a host-wrapped object
// that fails Array.isArray and whose elements aren't real JS strings, which
// breaks every extension that branches on those (e.g. titles[0].replace(...)).
func TestNativeArrayMarshaling(t *testing.T) {
	payload := `export default new class {
		async probe({ titles }) {
			return [{
				name: 'isArr:' + Array.isArray(titles) +
				      ' typeof0:' + (typeof titles[0]) +
				      ' replaced:' + titles[0].replace(/\./g, '_'),
				link: 'magnet:?xt=urn:btih:deadbeef'
			}]
		}
	}()`
	vm, err := NewVM("probe", payload, http.DefaultClient, testLogger(t))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := vm.CallMethod(context.Background(), "probe", map[string]interface{}{
		"titles": []string{"Dr. STONE", "Second"},
	})
	if err != nil {
		t.Fatal(err)
	}
	items, ok := raw.([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("expected 1 result, got %#v", raw)
	}
	m, ok := items[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %#v", items[0])
	}
	got, _ := m["name"].(string)
	want := "isArr:true typeof0:string replaced:Dr_ STONE"
	if got != want {
		t.Fatalf("native array marshaling broken:\n got: %q\nwant: %q", got, want)
	}
}
