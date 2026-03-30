package host

import (
	"context"
	"sync"
	"time"
)

type Registry struct {
	events chan Event

	provider    Provider
	connChecker ConnectionChecker
	hosts       map[Host]info

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewRegistry() *Registry {
	return &Registry{
		events: make(chan Event),
		hosts:  make(map[Host]info),
	}
}

func (r *Registry) Start(ctx context.Context) error {
	ctx, r.cancel = context.WithCancel(ctx)

	r.wg.Go(func() {
		// TODO(tzdybal): error handling
		_ = r.eventLoop(ctx)
	})

	if r.provider != nil {
		// TODO(tzdybal): error handling
		r.wg.Go(func() {
			_ = r.provider.Start(ctx, r.events)
		})
	}

	return nil
}

func (r *Registry) Close() error {
	if r.provider != nil {
		r.provider.Close()
	}
	r.cancel()
	r.wg.Wait()
	return nil
}

func (r *Registry) eventLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case e := <-r.events:
			r.handle(e)
		}
	}
}

func (r *Registry) handle(e Event) error {
	switch e.Type {
	case HostRegistered:
		r.hosts[e.Host] = info{}
	case HostDeregistered:
		delete(r.hosts, e.Host)
	case HostOnline:
	case HostOffline:
	default:
		// TODO(tzdybal): log
	}
	return nil
}

type Event struct {
	Type EventType
	Host Host
}

type Host string
type EventType int

const (
	HostRegistered EventType = iota
	HostDeregistered
	HostOnline
	HostOffline
)

type info struct {
	online    bool
	lastQuery time.Time
}
