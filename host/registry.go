package host

import (
	"context"
	"errors"
	"slices"
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
	monitors  map[Host]context.CancelFunc

	providersWG sync.WaitGroup
	monitorsWG  sync.WaitGroup

	hosts map[Host]*Info

	logger *zap.Logger
}

// Observer defines callbacks called by registry when host information is updated.
type Observer interface {
	Up(Host)
	Down(Host)
	CollectionsAdded(Host, []string)
	CollectionsRemoved(Host, []string)
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
		hosts:       make(map[Host]*Info),
		monitors:    make(map[Host]context.CancelFunc),
		logger:      logger.Named("Registry"),
		providers:   providers,
		observers:   observers,
		connChecker: connChecker,
		collFetcher: collFetcher,
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
	r.monitorsWG.Go(func() {
		r.monitor(ctx, h)
	})

	r.nofityHostUp(ctx, h)
}

func (r *Registry) deregister(ctx context.Context, h Host) {
	if cancel, ok := r.monitors[h]; ok {
		cancel()
		delete(r.monitors, h)

		r.nofityHostDown(ctx, h)
	}

}

func (r *Registry) monitor(ctx context.Context, h Host) {

}

func (r *Registry) nofityHostUp(ctx context.Context, h Host) {
	for _, o := range r.observers {
		select {
		case <-ctx.Done():
			break
		default:
			o.Up(h)
		}
	}
}
func (r *Registry) nofityHostDown(ctx context.Context, h Host) {
	for _, o := range r.observers {
		select {
		case <-ctx.Done():
			break
		default:
			o.Down(h)
		}
	}
}

// Wait blocks until all internal goroutines have stopped and returns the first non-nil error.
func (r *Registry) Wait() error {
	return nil
}

// Close cancels the registry context and closes all providers.
func (r *Registry) Close() error {
	return nil
}

// GetOnlineHosts returns information about all online hosts.
func (r *Registry) GetOnlineHosts() map[Host]*Info {
	online := make(map[Host]*Info)

	for h, i := range r.hosts {
		if i.GetOnline() {
			online[h] = i
		}
	}
	return online
}

func (r *Registry) startCollectionsWorker(ctx context.Context, h Host) {
	if r.collFetcher == nil {
		return
	}
	if _, running := r.monitors[h]; running {
		return
	}
	//workerCtx, cancel := context.WithCancel(ctx) //nolint:gosec // cancel is invoked by stopCollectionsWorker
	//r.collWorkers[h] = cancel
	//r.errGroup.Go(func() error {
	//	return r.collectionsWorker(workerCtx, h)
	//})
}

func (r *Registry) stopCollectionsWorker(h Host) {
	if cancel, ok := r.monitors[h]; ok {
		cancel()
		delete(r.monitors, h)
	}
}

func (r *Registry) collectionsWorker(ctx context.Context, h Host) error {
	r.fetchAndStoreCollections(ctx, h)

	ticker := time.NewTicker(r.config.CollectionsRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil
			}
			return ctx.Err()
		case <-ticker.C:
			r.fetchAndStoreCollections(ctx, h)
		}
	}
}

func (r *Registry) fetchAndStoreCollections(ctx context.Context, h Host) {
	names, err := r.collFetcher.FetchCollections(ctx, h)
	if err != nil {
		r.logger.Sugar().Debugw("collections fetch failed", "host", h, "error", err)
		return
	}
	if info, ok := r.hosts[h]; ok {
		info.SetCollections(names)
	}
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
			if res.Online {
				r.nofityHostUp(ctx, host)
			} else {
				r.nofityHostDown(ctx, host)
			}
		}
	}
}

// Info holds host information in thread safe way.
type Info struct {
	online bool
	// TODO(tzdybal): gather and use more information about hosts
	collections []string

	mtx sync.Mutex
}

// NewInfo creates new instance of Info object.
func NewInfo() *Info {
	return &Info{}
}

// SetOnline updates the online status, safe for concurrentuse.
func (i *Info) SetOnline(o bool) {
	i.mtx.Lock()
	defer i.mtx.Unlock()
	i.online = o
}

// GetOnline returns online status, safe for concurrent use.
func (i *Info) GetOnline() bool {
	i.mtx.Lock()
	defer i.mtx.Unlock()
	return i.online
}

// SetCollections sets the collections, safe for concurrent use.
func (i *Info) SetCollections(collections []string) {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	i.collections = collections
}

// GetCollections gets the collections, safe for concurrent use.
func (i *Info) GetCollections() []string {
	i.mtx.Lock()
	defer i.mtx.Unlock()

	return slices.Clone(i.collections)
}
