package host

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Host is the address (e.g. URL) of a network host.
type Host string

// Registry tracks known hosts and their connectivity status.
type Registry struct {
	config Config

	connChecker ConnectionChecker
	collFetcher CollectionsFetcher

	observers []Observer
	providers []Provider

	mtx      sync.Mutex
	monitors map[Host]context.CancelFunc

	providersWG sync.WaitGroup
	monitorsWG  sync.WaitGroup

	logger *zap.Logger
}

// Observer defines callbacks called by registry when host information is updated.
type Observer interface {
	Up(host Host)
	Down(host Host)
	CollectionsAdded(host Host, collections []string)
	CollectionsRemoved(host Host, collections []string)
}

// Config holds configuration for the Registry.
type Config struct {
	ConnCheckInterval          time.Duration
	CollectionsRefreshInterval time.Duration
}

// NewRegistry creates a new Registry with the given configuration, host providers, connection checker, and collections fetcher.
func NewRegistry(
	config Config,
	providers []Provider,
	observers []Observer,
	connChecker ConnectionChecker,
	collFetcher CollectionsFetcher,
	logger *zap.Logger,
) *Registry {
	return &Registry{
		config:      config,
		connChecker: connChecker,
		collFetcher: collFetcher,
		providers:   providers,
		observers:   observers,
		monitors:    make(map[Host]context.CancelFunc),
		logger:      logger.Named("Registry"),
	}
}

// Run launches all providers and begins processing host events.
func (r *Registry) Run(ctx context.Context) error {
	register := func(h Host) { r.register(ctx, h) }
	deregister := func(h Host) { r.deregister(ctx, h) }

	for _, provider := range r.providers {
		r.providersWG.Go(func() {
			if err := provider.Run(ctx, register, deregister); err != nil && !errors.Is(err, context.Canceled) {
				r.logger.Sugar().Errorw("provider exited", "error", err)
			}
		})
	}

	<-ctx.Done()
	// wait for providers first to make sure monitors are not started anymore
	r.providersWG.Wait()
	r.monitorsWG.Wait()
	return nil
}

func (r *Registry) register(ctx context.Context, h Host) {
	r.mtx.Lock()
	if _, ok := r.monitors[h]; ok {
		r.mtx.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(ctx) //nolint:gosec // false positive, fixed recently: https://github.com/securego/gosec/commit/e354c572d957eb8bf63481cc9ba2704b58a6ae35
	r.monitors[h] = cancel
	r.mtx.Unlock()

	r.monitorsWG.Go(func() {
		m := newMonitor(h, r.connChecker, r.collFetcher, r.observers, r.logger)

		m.run(ctx, r.config.ConnCheckInterval, r.config.CollectionsRefreshInterval)
	})
}

func (r *Registry) deregister(_ context.Context, h Host) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	if cancel, ok := r.monitors[h]; ok {
		delete(r.monitors, h)
		cancel()
	}
}
