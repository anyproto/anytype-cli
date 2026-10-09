package grpcserver

import (
	"context"
	"sync"
)

// observationOrder protects observation bookkeeping only, never Heart handler
// execution. Overlapping lifecycle calls must reach Heart to supersede/cancel
// one another. Their native effect order is not known to this observer, so the
// whole overlapping batch remains uncertain until a later independent call.
type observationOrder struct {
	mu      sync.Mutex
	active  int
	overlap bool
}

func (o *observationOrder) begin(ctx context.Context, observe func(bool)) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if o.active == 0 {
		o.overlap = false
	} else {
		o.overlap = true
	}
	o.active++
	observe(o.overlap)
	return nil
}

func (o *observationOrder) finish(observe func()) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.overlap {
		observe()
	}
	o.active--
}
