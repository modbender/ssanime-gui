package encode

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/modbender/ssanime-gui/internal/events"
	"github.com/modbender/ssanime-gui/internal/store"
)

// TestEncodeProgressWireContract locks the exact snake_case key set emitted on
// the encode.progress SSE event. The frontend (sse.svelte.ts EncodeProgress)
// reads these names verbatim; this fails loudly if any key drifts to camelCase.
func TestEncodeProgressWireContract(t *testing.T) {
	hub := events.NewHub(nil)
	sub := hub.Subscribe()
	defer sub.Close()

	q := &Queue{hub: hub}
	q.emitProgress(store.Episode{ID: 7, SeriesID: 3}, store.EncodedOutput{ID: 11}, 1080, 42.5, "1.2x")

	got := captureKeys(t, sub.Events())
	assertKeys(t, got, []string{
		"episode_id", "series_id", "output_id", "resolution", "percent", "speed",
	})
}

func captureKeys(t *testing.T, ch <-chan events.Event) map[string]struct{} {
	t.Helper()
	select {
	case ev := <-ch:
		raw, err := json.Marshal(ev.Data)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		keys := make(map[string]struct{}, len(m))
		for k := range m {
			keys[k] = struct{}{}
		}
		return keys
	case <-time.After(time.Second):
		t.Fatal("no event broadcast")
		return nil
	}
}

func assertKeys(t *testing.T, got map[string]struct{}, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("key count = %d, want %d; got %v", len(got), len(want), got)
	}
	for _, k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("missing key %q; got %v", k, got)
		}
	}
}
