package encode

import (
	"context"
	"testing"
)

// TestQueuePauseResume verifies that Pause/Resume flip the paused flag and that
// claim returns (zero, false) while paused, without touching the DB.
func TestQueuePauseResume(t *testing.T) {
	q := &Queue{}

	if q.Paused() {
		t.Fatal("queue should not be paused at construction")
	}

	q.Pause()
	if !q.Paused() {
		t.Fatal("queue should be paused after Pause()")
	}

	// claim must short-circuit on the paused flag before any store access.
	_, ok := q.claim(context.Background())
	if ok {
		t.Fatal("claim should return false while paused")
	}

	q.Resume()
	if q.Paused() {
		t.Fatal("queue should not be paused after Resume()")
	}
}
