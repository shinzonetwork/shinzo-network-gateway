package host

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// Registry tracks known hosts and their connectivity status.
type Registry struct {
	config Config
	events chan Event

	providers   []Provider
	connChecker ConnectionChecker
	hosts       map[Host]*Info

	cancel   context.CancelFunc
	errGroup *errgroup.Group

	logger *zap.Logger
}

// Config holds configuration for the Registry.
type Config struct {
	ConnCheckInterval time.Duration
}

// NewRegistry creates a new Registry with the given configuration, host providers, and connection checker.
func NewRegistry(config Config, providers []Provider, connChecker ConnectionChecker, logger *zap.Logger) *Registry {
	return &Registry{
		config:      config,
		events:      make(chan Event),
		hosts:       make(map[Host]*Info),
		logger:      logger.Named("Registry"),
		providers:   providers,
		connChecker: connChecker,
	}
}

// Start launches all providers and begins processing host events.
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

// Wait blocks until all internal goroutines have stopped and returns the first non-nil error.
func (r *Registry) Wait() error {
	return r.errGroup.Wait()
}

// Close cancels the registry context and closes all providers.
func (r *Registry) Close() error {
	r.cancel()
	var err error
	for _, provider := range r.providers {
		err = errors.Join(err, provider.Close())
	}
	return err
}

// GetOnlineHosts returns information about all online hosts.
func (r *Registry) GetOnlineHosts() map[Host]*Info {
	online := make(map[Host]*Info)

	for h, i := range r.hosts {
		if i.Online {
			online[h] = i
		}
	}
	return online
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
		r.hosts[e.Host] = &Info{}
		r.errGroup.Go(func() error {
			return r.connCheckerWorker(ctx, e.Host)
		})
	case HostDeregistered:
		// TODO(tzdybal): stop connection checker worker!
		delete(r.hosts, e.Host)
	case HostOnline:
		r.hosts[e.Host].Online = true
	case HostOffline:
		r.hosts[e.Host].Online = false
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

// Event represents a host lifecycle or connectivity change.
type Event struct {
	Type EventType
	Host Host
}

// Host is the address (e.g. URL) of a network host.
type Host string

// EventType identifies the kind of host event.
type EventType int

// Host event types.
const (
	HostRegistered   EventType = iota // host was registered in Shinzo Network
	HostDeregistered                  // host was deregistered from Shinzo Network
	HostOnline                        // host is reachable
	HostOffline                       // host is unreachable
)

type Info struct {
	Online bool
	// TODO(tzdybal): gather and use more information about hosts
	Collections []string
}
