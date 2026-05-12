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
	mtx   sync.RWMutex

	logger *zap.Logger
}

var (
	_ endpoint.HostsSelector = &Router{}
	_ host.Observer          = &Router{}
)

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

	r.mtx.RLock()
	defer r.mtx.RUnlock()

	col := collections[0]
	pool, ok := r.pools[col]
	if !ok {
		return nil, ErrPoolNotFound
	}

	// return configured number of hosts, or all of them
	l := min(sampleSize, len(pool.hosts.Pool()))
	return pool.get(l)
}

func (r *Router) Up(_ host.Host) {
	// no-op
}

func (r *Router) Down(h host.Host) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	for _, p := range r.pools {
		p.remove(h)
	}
}

func (r *Router) CollectionsAdded(h host.Host, colls []string) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	for _, c := range colls {
		p, ok := r.pools[c]
		if !ok {
			p = newPool(c, nil, r.logger)
			r.pools[c] = p
		}
		p.add(h)
	}
}

func (r *Router) CollectionsRemoved(h host.Host, colls []string) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	for _, c := range colls {
		if p, ok := r.pools[c]; ok {
			p.remove(h)
		}
	}
}
