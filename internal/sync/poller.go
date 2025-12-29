package sync

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type Poller struct {
	Interval time.Duration
	Refresh  func(ctx context.Context) error
	Logger   *slog.Logger

	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
}

func (p *Poller) Start(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running {
		return
	}
	p.running = true
	p.stopCh = make(chan struct{})

	go func() {
		ticker := time.NewTicker(p.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if p.Refresh == nil {
					continue
				}
				if err := p.Refresh(ctx); err != nil && p.Logger != nil {
					p.Logger.Warn("poll refresh failed", "error", err)
				}
			case <-p.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (p *Poller) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running {
		return
	}
	close(p.stopCh)
	p.running = false
}
