package router

import (
	"context"
	"errors"
	"sync"

	"github.com/shinzonetwork/shinzo-network-gateway/endpoint"
	"github.com/shinzonetwork/shinzo-network-gateway/host"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

var (
	// ErrPoolNotSupported is returned if requested type of pool is not supported by Router.
	ErrPoolNotSupported = errors.New("queries spanning multiple collections are not supported")
	// ErrPoolNotFound is returned when pool was not found by the Router.
	ErrPoolNotFound = errors.New("no hosts serve the requested collection")
)

// Router selects the hosts for query based on information from Registry.
type Router struct {
	pools map[string]*pool
	mtx   sync.RWMutex

	logger *zap.Logger
}

var (
	_ endpoint.HostsSelector = &Router{}
	_ host.Observer          = &Router{}
)

// New creates new instance of Router.
func New(logger *zap.Logger) *Router {
	r := &Router{
		pools:  make(map[string]*pool),
		logger: logger.Named("router"),
	}

	meter := otel.Meter("github.com/shinzonetwork/shinzo-network-gateway/router")
	_, err := meter.Int64ObservableGauge("shinzo.gateway.hosts.routable",
		metric.WithDescription("Online hosts serving at least one collection."),
		metric.WithUnit("{host}"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			hosts, _ := r.routable()
			o.Observe(int64(hosts))
			return nil
		}),
	)
	if err != nil {
		otel.Handle(err)
	}
	_, err = meter.Int64ObservableGauge("shinzo.gateway.collections",
		metric.WithDescription("Collections served by at least one online host."),
		metric.WithUnit("{collection}"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			_, collections := r.routable()
			o.Observe(int64(collections))
			return nil
		}),
	)
	if err != nil {
		otel.Handle(err)
	}

	return r
}

// routable returns the number of distinct hosts in all pools and the number of collections with at least one host.
func (r *Router) routable() (hosts, collections int) {
	r.mtx.RLock()
	pools := make([]*pool, 0, len(r.pools))
	for _, p := range r.pools {
		pools = append(pools, p)
	}
	r.mtx.RUnlock()

	seen := make(map[host.Host]struct{})
	for _, p := range pools {
		if p.addHostsTo(seen) > 0 {
			collections++
		}
	}
	return len(seen), collections
}

// SelectHosts finds the hosts that serve all the collections, and return random sample.
func (r *Router) SelectHosts(_ context.Context, n int, collections []string) ([]host.Host, error) {
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

	// return requested number of hosts, or all of them
	l := min(n, len(pool.hosts.Pool()))
	return pool.get(l)
}

// Up implements host.Observer; the Router does not track hosts until they advertise collections.
func (r *Router) Up(_ host.Host) {
	// no-op
}

// Down implements host.Observer by removing the host from every pool it belongs to.
func (r *Router) Down(h host.Host) {
	r.logger.Debug("removing host from all pools", zap.String("host", string(h)))
	r.mtx.Lock()
	defer r.mtx.Unlock()
	for _, p := range r.pools {
		p.remove(h)
	}
}

// CollectionsAdded implements host.Observer by adding the host to the pool for each given collection.
func (r *Router) CollectionsAdded(h host.Host, colls []string) {
	r.logger.Debug("adding host to pools", zap.String("host", string(h)), zap.Strings("collections", colls))
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

// CollectionsRemoved implements host.Observer by removing the host from the pool for each given collection.
func (r *Router) CollectionsRemoved(h host.Host, colls []string) {
	r.logger.Debug("removing host from pools", zap.String("host", string(h)), zap.Strings("collections", colls))
	r.mtx.Lock()
	defer r.mtx.Unlock()
	for _, c := range colls {
		if p, ok := r.pools[c]; ok {
			p.remove(h)
		}
	}
}
