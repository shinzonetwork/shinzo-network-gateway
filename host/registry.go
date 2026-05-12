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

	mtx      sync.Mutex
	monitors map[Host]context.CancelFunc

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
	r.mtx.Lock()
	if _, ok := r.monitors[h]; ok {
		r.mtx.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	r.monitors[h] = cancel
	r.mtx.Unlock()

	r.monitorsWG.Go(func() {
		r.monitor(ctx, h)
	})
}

func (r *Registry) deregister(ctx context.Context, h Host) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	if cancel, ok := r.monitors[h]; ok {
		cancel()
	}
}

func (r *Registry) monitor(ctx context.Context, h Host) {
	var (
		online bool
		colls  []string
	)

	checkConn := func() {
		res := r.connChecker.CheckConnection(ctx, h)
		if res.Online != online {
			if res.Online {
				r.notifyHostUp(h)
			} else {
				r.notifyHostDown(h)
			}
			online = res.Online
		}
	}

	// start checking status immediately
	checkConn()

	connTicker := time.NewTicker(r.config.ConnCheckInterval)
	defer connTicker.Stop()
	collTicker := time.NewTicker(r.config.CollectionsRefreshInterval)
	defer collTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			delete(r.monitors, h)
			if online {
				r.notifyCollsUpdate(h, colls, nil)
				r.notifyHostDown(h)
			}
			return
		case <-connTicker.C:
			checkConn()
		case <-collTicker.C:
			if !online {
				continue
			}
			newColls, err := r.collFetcher.FetchCollections(ctx, h)
			if err != nil {
				r.logger.Sugar().Errorw("error while fetching collections", "host", string(h), "error", err)
				continue
			}
			slices.Sort(newColls)
			r.notifyCollsUpdate(h, colls, newColls)
			colls = newColls
		}
	}
}

func (r *Registry) notifyHostUp(h Host) {
	for _, o := range r.observers {
		o.Up(h)
	}
}

func (r *Registry) notifyHostDown(h Host) {
	for _, o := range r.observers {
		o.Down(h)
	}
}

// notifyCollsUpdate compares oldColls with newCools, prepares a list of removed collections and list of added collections
// It assumes that oldColls and newColls are sorted.
func (r *Registry) notifyCollsUpdate(h Host, oldColls, newColls []string) {
	added, removed := getSliceDiffs(oldColls, newColls)

	for _, o := range r.observers {
		if len(added) > 0 {
			o.CollectionsAdded(h, added)
		}
		if len(removed) > 0 {
			o.CollectionsRemoved(h, removed)
		}
	}
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
