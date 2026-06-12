package events

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const (
	// clientBuffer is the per-client send queue depth. A client that doesn't
	// drain within this many pending events is considered stalled.
	clientBuffer = 64
	// defaultHeartbeat is how often the hub emits a heartbeat keep-alive,
	// which also doubles as the stalled-client garbage collector trigger.
	defaultHeartbeat = 15 * time.Second
)

// client is one subscribed SSE connection. dropped is closed by the hub when the
// client falls behind, so the serving handler can tear the connection down.
type client struct {
	ch      chan Event
	dropped chan struct{}
}

// Hub is the SSE pub/sub manager. It owns the set of subscribed clients and a
// heartbeat ticker. Broadcast fans an event out to every client non-blockingly:
// a client whose buffer is full is dropped rather than blocking the producer.
type Hub struct {
	logger    *slog.Logger
	heartbeat time.Duration

	mu      sync.RWMutex
	clients map[*client]struct{}

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// Option configures a Hub.
type Option func(*Hub)

// WithHeartbeat overrides the heartbeat interval.
func WithHeartbeat(d time.Duration) Option {
	return func(h *Hub) {
		if d > 0 {
			h.heartbeat = d
		}
	}
}

// NewHub constructs a hub. Call Start to begin the heartbeat ticker.
func NewHub(logger *slog.Logger, opts ...Option) *Hub {
	if logger == nil {
		logger = slog.Default()
	}
	h := &Hub{
		logger:    logger,
		heartbeat: defaultHeartbeat,
		clients:   make(map[*client]struct{}),
		done:      make(chan struct{}),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Start launches the heartbeat loop. It is safe to call once; Stop ends it.
func (h *Hub) Start() {
	h.ctx, h.cancel = context.WithCancel(context.Background())
	go h.run()
}

func (h *Hub) run() {
	defer close(h.done)
	ticker := time.NewTicker(h.heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-h.ctx.Done():
			return
		case t := <-ticker.C:
			h.Broadcast(TypeHeartbeat, map[string]int64{"ts": t.Unix()})
		}
	}
}

// Stop ends the heartbeat loop and closes every client channel. After Stop the
// hub broadcasts nothing further.
func (h *Hub) Stop() {
	if h.cancel != nil {
		h.cancel()
		<-h.done
	}
	h.mu.Lock()
	for c := range h.clients {
		close(c.ch)
		delete(h.clients, c)
	}
	h.mu.Unlock()
}

// subscriber is the read side of a subscription handed to a serving handler.
type subscriber struct {
	hub *Hub
	c   *client
}

// Events is the channel of events for this subscriber. It is closed when the
// subscriber is dropped (slow client) or the hub stops.
func (s *subscriber) Events() <-chan Event { return s.c.ch }

// Dropped is closed if the hub drops this subscriber for being too slow.
func (s *subscriber) Dropped() <-chan struct{} { return s.c.dropped }

// Close unregisters the subscriber and releases its resources. Idempotent.
func (s *subscriber) Close() { s.hub.unsubscribe(s.c) }

// Subscribe registers a new client and returns its subscriber handle. The caller
// must Close it (typically via defer) when the connection ends.
func (h *Hub) Subscribe() *subscriber {
	c := &client{
		ch:      make(chan Event, clientBuffer),
		dropped: make(chan struct{}),
	}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	return &subscriber{hub: h, c: c}
}

// unsubscribe removes a client and closes its channel exactly once.
func (h *Hub) unsubscribe(c *client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.ch)
	}
	h.mu.Unlock()
}

// Broadcast fans an event out to every subscribed client. A client whose buffer
// is full is dropped (its dropped channel is closed and it is unregistered) so a
// stalled consumer can never block the producer.
func (h *Hub) Broadcast(t Type, data any) {
	ev := Event{Type: t, Data: data}
	var slow []*client

	h.mu.RLock()
	for c := range h.clients {
		select {
		case c.ch <- ev:
		default:
			slow = append(slow, c)
		}
	}
	h.mu.RUnlock()

	for _, c := range slow {
		h.drop(c)
	}
}

// drop unregisters a stalled client, signaling it via dropped and closing its
// channel. Done outside the broadcast read-lock to avoid lock upgrade.
func (h *Hub) drop(c *client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.dropped)
		close(c.ch)
		h.logger.Warn("events: dropped slow SSE client")
	}
	h.mu.Unlock()
}

// ClientCount returns the number of currently subscribed clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
