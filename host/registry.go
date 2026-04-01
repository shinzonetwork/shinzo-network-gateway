package host

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

type Registry struct {
	events chan Event

	providers   []Provider
	connChecker ConnectionChecker
	hosts       map[Host]info

	cancel   context.CancelFunc
	errGroup *errgroup.Group

	logger *zap.Logger
}

func NewRegistry(providers []Provider, connChecker ConnectionChecker, logger *zap.Logger) *Registry {
	return &Registry{
		events:      make(chan Event),
		hosts:       make(map[Host]info),
		logger:      logger.Named("Registry"),
		providers:   providers,
		connChecker: connChecker,
	}
}

func (r *Registry) Start(ctx context.Context) error {
	ctx, r.cancel = context.WithCancel(ctx)
	r.errGroup, ctx = errgroup.WithContext(ctx)

	r.errGroup.Go(func() error {
		return r.eventLoop(ctx)
	})

	for _, provider := range r.providers {
		r.errGroup.Go(func() error {
			return provider.Start(ctx, r.events)
		})
	}

	return nil
}

func (r *Registry) Close() error {
	r.cancel()
	var err error
	for _, provider := range r.providers {
		err = errors.Join(err, provider.Close())
	}
	return errors.Join(err, r.errGroup.Wait())
}

func (r *Registry) eventLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil
			}
			return ctx.Err()
		case e := <-r.events:
			err := r.handle(e)
			if err != nil {
				r.logger.Sugar().Debugw("error while handling event", "error", err)
			}
		}
	}
}

func (r *Registry) handle(e Event) error {
	r.logger.Sugar().Debugw("event received", "event", e)
	switch e.Type {
	case HostRegistered:
		_, found := r.hosts[e.Host]
		if found {
			r.logger.Sugar().Infow("host already registered", "address", e.Host)
			return nil
		}
		r.hosts[e.Host] = info{}
	case HostDeregistered:
		delete(r.hosts, e.Host)
	case HostOnline:
	case HostOffline:
	default:
		r.logger.Sugar().Errorw("unknown event type", "type", e.Type)
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
