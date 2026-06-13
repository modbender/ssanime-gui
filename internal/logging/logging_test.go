package logging

import (
	"strings"
	"testing"
	"time"

	"github.com/modbender/ssanime-gui/internal/events"
)

// drain reads one event within a short window, or returns ok=false if none arrives.
func drain(sub interface{ Events() <-chan events.Event }) (events.Event, bool) {
	select {
	case ev := <-sub.Events():
		return ev, true
	case <-time.After(200 * time.Millisecond):
		return events.Event{}, false
	}
}

func TestBridgeWarnBroadcastsFrozenShape(t *testing.T) {
	logger, bridge, _, closer := Build(t.TempDir())
	defer closer.Close()

	hub := events.NewHub(nil, events.WithHeartbeat(time.Hour)) // suppress heartbeat noise
	hub.Start()
	defer hub.Stop()
	bridge.Attach(hub)

	sub := hub.Subscribe()
	defer sub.Close()

	logger.Warn("disk full", "path", "/tmp/x", "code", 28)

	ev, ok := drain(sub)
	if !ok {
		t.Fatal("expected one TypeLog broadcast for a Warn record")
	}
	if ev.Type != events.TypeLog {
		t.Fatalf("event type = %q, want %q", ev.Type, events.TypeLog)
	}
	p, ok := ev.Data.(logPayload)
	if !ok {
		t.Fatalf("event data type = %T, want logPayload", ev.Data)
	}
	if p.Level != "warn" {
		t.Errorf("level = %q, want warn", p.Level)
	}
	if p.Message != "disk full path=/tmp/x code=28" {
		t.Errorf("message = %q, want attrs appended", p.Message)
	}
	if p.TS <= 0 || p.TS > time.Now().Unix()+2 {
		t.Errorf("ts = %d, want plausible unix seconds", p.TS)
	}

	// Exactly one: no second broadcast trailing the same record.
	if _, ok := drain(sub); ok {
		t.Error("expected exactly one broadcast, got a second")
	}
}

func TestBridgeDebugProducesNothing(t *testing.T) {
	logger, bridge, _, closer := Build(t.TempDir())
	defer closer.Close()

	hub := events.NewHub(nil, events.WithHeartbeat(time.Hour))
	hub.Start()
	defer hub.Stop()
	bridge.Attach(hub)

	sub := hub.Subscribe()
	defer sub.Close()

	logger.Debug("noisy detail", "k", "v")

	if ev, ok := drain(sub); ok {
		t.Fatalf("Debug should not broadcast, got %+v", ev)
	}
}

func TestBridgePreAttachProducesNothing(t *testing.T) {
	logger, _, _, closer := Build(t.TempDir())
	defer closer.Close()

	// A separate hub subscribed but never attached to the bridge must see nothing.
	hub := events.NewHub(nil, events.WithHeartbeat(time.Hour))
	hub.Start()
	defer hub.Stop()
	sub := hub.Subscribe()
	defer sub.Close()

	logger.Info("before attach")

	if ev, ok := drain(sub); ok {
		t.Fatalf("pre-attach Info should not broadcast, got %+v", ev)
	}
}

// TestRingCapturesLoggedLines verifies the ring is wired as a sink on the text
// handler: a line written through the logger appears, fully formatted, in
// ring.Lines() — this is what backs GET /api/logs' historic section.
func TestRingCapturesLoggedLines(t *testing.T) {
	logger, _, ring, closer := Build(t.TempDir())
	defer closer.Close()

	logger.Info("ring capture", "key", "val")

	lines := ring.Lines(0)
	if len(lines) != 1 {
		t.Fatalf("ring.Lines() len = %d, want 1: %#v", len(lines), lines)
	}
	got := lines[0]
	if !strings.Contains(got, "msg=\"ring capture\"") || !strings.Contains(got, "key=val") {
		t.Errorf("ring line missing formatted fields: %q", got)
	}
	if strings.Contains(got, "\n") {
		t.Errorf("ring line should not contain a newline: %q", got)
	}
}

// TestBridgeNoRecursionOnHubInternalLog guards the feedback loop: the hub's only
// on-broadcast log is drop()'s "events: ..." Warn. Routing that record back into
// the bridge would loop. The bridge skips the hubInternalPrefix, so such a record
// produces no broadcast.
func TestBridgeNoRecursionOnHubInternalLog(t *testing.T) {
	logger, bridge, _, closer := Build(t.TempDir())
	defer closer.Close()

	hub := events.NewHub(nil, events.WithHeartbeat(time.Hour))
	hub.Start()
	defer hub.Stop()
	bridge.Attach(hub)

	sub := hub.Subscribe()
	defer sub.Close()

	logger.Warn("events: dropped slow SSE client")

	if ev, ok := drain(sub); ok {
		t.Fatalf("hub-internal record must not re-broadcast, got %+v", ev)
	}
}
