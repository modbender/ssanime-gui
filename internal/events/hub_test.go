package events

import (
	"testing"
	"time"
)

func TestSubscribeBroadcastReceive(t *testing.T) {
	h := NewHub(nil)
	sub := h.Subscribe()
	defer sub.Close()

	if got := h.ClientCount(); got != 1 {
		t.Fatalf("ClientCount = %d, want 1", got)
	}

	h.Broadcast(TypeLog, map[string]string{"msg": "hello"})

	select {
	case ev := <-sub.Events():
		if ev.Type != TypeLog {
			t.Fatalf("event type = %q, want %q", ev.Type, TypeLog)
		}
		data, ok := ev.Data.(map[string]string)
		if !ok || data["msg"] != "hello" {
			t.Fatalf("event data = %v, want msg=hello", ev.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive broadcast event")
	}
}

func TestUnsubscribeCleansUp(t *testing.T) {
	h := NewHub(nil)
	sub := h.Subscribe()

	if got := h.ClientCount(); got != 1 {
		t.Fatalf("ClientCount = %d, want 1", got)
	}

	sub.Close()
	if got := h.ClientCount(); got != 0 {
		t.Fatalf("after Close ClientCount = %d, want 0", got)
	}

	// Channel must be closed after unsubscribe.
	select {
	case _, ok := <-sub.Events():
		if ok {
			t.Fatal("expected closed channel after Close")
		}
	case <-time.After(time.Second):
		t.Fatal("channel not closed after Close")
	}

	// Close is idempotent.
	sub.Close()
}

func TestBroadcastDropsSlowClient(t *testing.T) {
	h := NewHub(nil)
	sub := h.Subscribe()
	defer sub.Close()

	// Overfill the buffer without draining: the (clientBuffer+1)th send drops.
	for i := 0; i < clientBuffer+5; i++ {
		h.Broadcast(TypeLog, i)
	}

	select {
	case <-sub.Dropped():
	case <-time.After(time.Second):
		t.Fatal("slow client was not dropped")
	}
	if got := h.ClientCount(); got != 0 {
		t.Fatalf("dropped client still registered: ClientCount = %d", got)
	}
}

func TestHeartbeat(t *testing.T) {
	h := NewHub(nil, WithHeartbeat(10*time.Millisecond))
	h.Start()
	defer h.Stop()

	sub := h.Subscribe()
	defer sub.Close()

	select {
	case ev := <-sub.Events():
		if ev.Type != TypeHeartbeat {
			t.Fatalf("event type = %q, want %q", ev.Type, TypeHeartbeat)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive heartbeat")
	}
}

func TestStopClosesClients(t *testing.T) {
	h := NewHub(nil)
	h.Start()
	sub := h.Subscribe()

	h.Stop()

	select {
	case _, ok := <-sub.Events():
		if ok {
			t.Fatal("expected closed channel after Stop")
		}
	case <-time.After(time.Second):
		t.Fatal("channel not closed after Stop")
	}
}
