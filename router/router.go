package router

import (
	"context"
	"errors"
	"sync"

	"github.com/shinzonetwork/shinzo-network-gateway/endpoint"
	"github.com/shinzonetwork/shinzo-network-gateway/host"
	"go.uber.org/zap"
)

const (
	// TODO(tzdybal): this has to be configurable.
	sampleSize = 3
)

var (
	// ErrPoolNotSupported is returned if requested type of pool is not supported by Router.
	ErrPoolNotSupported = errors.New("only single-collection pools are supported")
	// ErrPoolNotFound is returned when pool was not found by the Router.
	ErrPoolNotFound = errors.New("pool not found")
)

// Router selects the hosts for query based on information from Registry.
type Router struct {
	registry *host.Registry

	pools map[string]*pool
	mtx   sync.Mutex

	logger *zap.Logger
}

var _ endpoint.HostsSelector = &Router{}

// New creates new instance of Router.
func New(registry *host.Registry, logger *zap.Logger) *Router {
	r := &Router{
		registry: registry,
		pools:    make(map[string]*pool),
		logger:   logger.Named("Router"),
	}

	return r
}

// SelectHosts finds the hosts that serve all the collections, and return random sample.
func (r *Router) SelectHosts(_ context.Context, collections []string) ([]host.Host, error) {
	if len(collections) != 1 {
		return nil, ErrPoolNotSupported
	}

	// TODO(tzdybal): this is absolutely not efficient, but needs bigger refactoring
	hosts := r.registry.GetOnlineHosts()
	for h, i := range hosts {
		for _, c := range i.GetCollections() {
			pool, ok := r.pools[c]
			if !ok {
				pool = newPool(c, nil, r.logger)
				r.pools[c] = pool
			}
			pool.add(h)
		}
	}

	col := collections[0]
	r.logger.Sugar().Debugf("searching for a pool for collection: %s", col)
	r.mtx.Lock()
	defer r.mtx.Unlock()
	pool, ok := r.pools[col]
	if !ok {
		return nil, ErrPoolNotFound
	}

	// return configured number of hosts, or all of them
	l := min(sampleSize, len(pool.hosts.Pool()))
	return pool.get(l)
}
