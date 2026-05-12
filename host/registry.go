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
	if _, ok := r.monitors[h]; ok {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	r.monitors[h] = cancel

	r.monitorsWG.Go(func() {
		r.monitor(ctx, h)
	})

	r.notifyHostUp(ctx, h)
}

func (r *Registry) deregister(ctx context.Context, h Host) {
	if cancel, ok := r.monitors[h]; ok {
		cancel()
		delete(r.monitors, h)

		r.notifyHostDown(ctx, h)
	}
}

func (r *Registry) monitor(ctx context.Context, h Host) {
	var (
		online bool
		colls  []string
	)

	connTicker := time.NewTicker(r.config.ConnCheckInterval)
	collTicker := time.NewTicker(r.config.CollectionsRefreshInterval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-connTicker.C:
			res := r.connChecker.CheckConnection(ctx, h)
			if res.Online != online {
				if res.Online == true {
					r.notifyHostUp(ctx, h)
				} else {
					r.notifyHostDown(ctx, h)
				}
				online = res.Online
			}
		case <-collTicker.C:
			newColls, err := r.collFetcher.FetchCollections(ctx, h)
			if err != nil {
				r.logger.Sugar().Errorw("error while fetching collections", "host", string(h), "error", err)
				break
			}
			slices.Sort(newColls)
			r.notifyCollsUpdate(ctx, h, colls, newColls)
			colls = newColls
		}
	}
}

func (r *Registry) notifyHostUp(ctx context.Context, h Host) {
	for _, o := range r.observers {
		select {
		case <-ctx.Done():
			return
		default:
			o.Up(h)
		}
	}
}
func (r *Registry) notifyHostDown(ctx context.Context, h Host) {
	for _, o := range r.observers {
		select {
		case <-ctx.Done():
			return
		default:
			o.Down(h)
		}
	}
}

// notifyCollsUpdate compares oldColls with newCools, prepares a list of removed collections and list of added collections
// It assumes that oldColls and newColls are sorted.
func (r *Registry) notifyCollsUpdate(ctx context.Context, h Host, oldColls, newColls []string) {

	added, removed := getSliceDiffs(oldColls, newColls)

	for _, o := range r.observers {
		select {
		case <-ctx.Done():
			return
		default:
			if len(added) > 0 {
				o.CollectionsAdded(h, added)
			}
			if len(removed) > 0 {
				o.CollectionsRemoved(h, removed)
			}
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

func getSliceDiffs(prev, next []string) (added []string, removed []string) {
	i, j := 0, 0
	for i < len(prev) && j < len(next) {
		switch {
		case prev[i] < next[j]:
			removed = append(removed, prev[i])
			i++
		case prev[i] > next[j]:
			added = append(added, next[j])
			j++
		default:
			i++
			j++
		}
	}
	removed = append(removed, prev[i:]...)
	added = append(added, next[j:]...)

	return
}
