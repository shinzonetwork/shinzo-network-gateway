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

type monitor struct {
	h      Host
	online bool
	colls  []string

	connChecker ConnectionChecker
	collFetcher CollectionsFetcher
	observers   []Observer

	logger *zap.Logger
}

func newMonitor(h Host, connChecker ConnectionChecker, collFetcher CollectionsFetcher, observers []Observer, logger *zap.Logger) *monitor {
	return &monitor{
		h:           h,
		connChecker: connChecker,
		collFetcher: collFetcher,
		observers:   observers,
		logger:      logger.With(zap.String("host", string(h))),
	}
}

func (m *monitor) checkColls(ctx context.Context) {
	newColls, err := m.collFetcher.FetchCollections(ctx, m.h)
	if err != nil {
		m.logger.Sugar().Errorw("error while fetching collections", "error", err)
		return
	}
	slices.Sort(newColls)
	m.notifyCollsUpdate(m.colls, newColls)
	m.colls = newColls
}

func (m *monitor) checkConn(ctx context.Context) {
	res := m.connChecker.CheckConnection(ctx, m.h)
	if res.Online != m.online {
		if res.Online {
			m.notifyHostUp()
			// fetch collections immediately
			m.checkColls(ctx)
		} else {
			m.notifyCollsUpdate(m.colls, nil)
			m.notifyHostDown()
			m.colls = nil
		}
		m.online = res.Online
	}
}

func (m *monitor) run(ctx context.Context, connCheckInterval, collectionsRefreshInterval time.Duration) {
	// start checking status immediately
	m.checkConn(ctx)

	connTicker := time.NewTicker(connCheckInterval)
	defer connTicker.Stop()
	collTicker := time.NewTicker(collectionsRefreshInterval)
	defer collTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			if m.online {
				m.notifyCollsUpdate(m.colls, nil)
				m.notifyHostDown()
			}
			return
		case <-connTicker.C:
			m.checkConn(ctx)
		case <-collTicker.C:
			if m.online {
				m.checkColls(ctx)
			}
		}
	}
}

func (m *monitor) notifyHostUp() {
	for _, o := range m.observers {
		o.Up(m.h)
	}
}

func (m *monitor) notifyHostDown() {
	for _, o := range m.observers {
		o.Down(m.h)
	}
}

// notifyCollsUpdate compares oldColls with newCools, prepares a list of removed collections and list of added collections
// It assumes that oldColls and newColls are sorted.
func (m *monitor) notifyCollsUpdate(oldColls, newColls []string) {
	added, removed := getSliceDiffs(oldColls, newColls)

	for _, o := range m.observers {
		if len(added) > 0 {
			o.CollectionsAdded(m.h, added)
		}
		if len(removed) > 0 {
			o.CollectionsRemoved(m.h, removed)
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
