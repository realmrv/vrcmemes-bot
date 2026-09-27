package bot

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mymmrac/telego"
)

type blockingLimiter struct {
	active atomic.Int64
	max    atomic.Int64
	gate   <-chan struct{}
}

func (l *blockingLimiter) Take() time.Time {
	n := l.active.Add(1)
	for peak := l.max.Load(); n > peak; peak = l.max.Load() {
		if l.max.CompareAndSwap(peak, n) {
			break
		}
	}
	<-l.gate
	l.active.Add(-1)
	return time.Now()
}

func TestStartBoundsHandlersAndReportsClosedChannel(t *testing.T) {
	updates := make(chan telego.Update, 40)
	for range 40 {
		updates <- telego.Update{}
	}
	close(updates)

	gate := make(chan struct{})
	limiter := &blockingLimiter{gate: gate}
	b := &Bot{updatesChan: updates, ratelimiter: limiter}
	done := make(chan error, 1)
	go func() { done <- b.Start(context.Background()) }()

	deadline := time.After(2 * time.Second)
	for limiter.active.Load() < 20 {
		select {
		case <-deadline:
			close(gate)
			t.Fatal("update handlers did not reach the configured limit")
		case <-time.After(time.Millisecond):
		}
	}
	if got := limiter.max.Load(); got != 20 {
		close(gate)
		t.Fatalf("maximum concurrent handlers = %d, want 20", got)
	}
	close(gate)
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "closed unexpectedly") {
			t.Fatalf("Start() error = %v, want unexpected channel closure", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start() did not stop after updates channel closed")
	}
	if got := limiter.max.Load(); got > 20 {
		t.Fatalf("maximum concurrent handlers = %d, want at most 20", got)
	}
}
