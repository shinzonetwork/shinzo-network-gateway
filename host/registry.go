package host

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

type Registry struct {
	config Config
	events chan Event

	providers   []Provider
	connChecker ConnectionChecker
	hosts       map[Host]*info

	cancel   context.CancelFunc
	errGroup *errgroup.Group

	logger *zap.Logger
}

// TODO(tzdybal): to be refactored
type Config struct {
	ConnCheckInterval time.Duration
}

var defaultConfig Config = Config{
	ConnCheckInterval: 5 * time.Second,
}

func NewRegistry(config Config, providers []Provider, connChecker ConnectionChecker, logger *zap.Logger) *Registry {
	return &Registry{
		config:      config,
		events:      make(chan Event),
		hosts:       make(map[Host]*info),
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
			err := r.handle(ctx, e)
			if err != nil {
				r.logger.Sugar().Debugw("error while handling event", "error", err)
			}
		}
	}
}

func (r *Registry) handle(ctx context.Context, e Event) error {
	r.logger.Sugar().Debugw("event received", "event", e)
	switch e.Type {
	case HostRegistered:
		_, found := r.hosts[e.Host]
		if found {
			r.logger.Sugar().Infow("host already registered", "address", e.Host)
			return nil
		}
		r.hosts[e.Host] = &info{}
		r.errGroup.Go(func() error {
			return r.connCheckerWorker(ctx, e.Host)
		})
	case HostDeregistered:
		// TODO(tzdybal): stop connection checker worker!
		delete(r.hosts, e.Host)
	case HostOnline:
		r.hosts[e.Host].online = true
	case HostOffline:
		r.hosts[e.Host].online = false
	default:
		r.logger.Sugar().Errorw("unknown event type", "type", e.Type)
	}
	return nil
}

func (r *Registry) connCheckerWorker(ctx context.Context, host Host) error {
	ticker := time.NewTicker(r.config.ConnCheckInterval)
	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil
			}
			return ctx.Err()
		case <-ticker.C:
			res := r.connChecker.CheckConnection(ctx, host)
			t := HostOnline
			if !res.Online {
				t = HostOffline
				// TODO(tzdybal): exponential backoff?
			}
			r.events <- Event{Type: t, Host: host}
		}
	}
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
