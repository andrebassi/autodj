// Package ws is the WebSocket driving adapter. It pushes now-playing + queue
// snapshots to connected Player UIs so the dashboard updates live.
package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"autodj/ports/inbound"
)

// Hub polls the PlaybackService and fans out snapshots to WS clients.
type Hub struct {
	play     inbound.PlaybackService
	interval time.Duration

	mu    sync.Mutex
	conns map[*websocket.Conn]struct{}
}

// NewHub builds a Hub polling every interval (default 2s if zero).
func NewHub(play inbound.PlaybackService, interval time.Duration) *Hub {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	return &Hub{play: play, interval: interval, conns: map[*websocket.Conn]struct{}{}}
}

// Run polls until ctx is cancelled, broadcasting each snapshot.
func (h *Hub) Run(ctx context.Context) {
	t := time.NewTicker(h.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.broadcast(ctx)
		}
	}
}

func (h *Hub) broadcast(ctx context.Context) {
	np, q, err := h.play.NowPlaying(ctx)
	if err != nil {
		return
	}
	msg := snapshot(np, q)
	payload, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.mu.Lock()
	conns := make([]*websocket.Conn, 0, len(h.conns))
	for c := range h.conns {
		conns = append(conns, c)
	}
	h.mu.Unlock()
	for _, c := range conns {
		wctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		if err := c.Write(wctx, websocket.MessageText, payload); err != nil {
			h.drop(c)
		}
		cancel()
	}
}

func (h *Hub) add(c *websocket.Conn) {
	h.mu.Lock()
	h.conns[c] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) drop(c *websocket.Conn) {
	h.mu.Lock()
	delete(h.conns, c)
	h.mu.Unlock()
}

// ServeHTTP upgrades the connection and keeps it open (server pushes only).
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	h.add(c)
	defer func() {
		h.drop(c)
		_ = c.Close(websocket.StatusNormalClosure, "bye")
	}()
	// Send an immediate snapshot so the UI isn't blank until the first tick.
	if np, q, err := h.play.NowPlaying(r.Context()); err == nil {
		if payload, err := json.Marshal(snapshot(np, q)); err == nil {
			_ = c.Write(r.Context(), websocket.MessageText, payload)
		}
	}
	// Block reading until the client disconnects.
	for {
		if _, _, err := c.Read(r.Context()); err != nil {
			return
		}
	}
}
