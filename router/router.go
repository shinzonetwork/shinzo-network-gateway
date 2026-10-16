package router

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"

	"github.com/shinzonetwork/shinzo-network-gateway/endpoint"
	"github.com/shinzonetwork/shinzo-network-gateway/host"
	"go.uber.org/zap"
)

var (
	// ErrPoolNotSupported is returned if requested type of pool is not supported by Router.
	ErrPoolNotSupported = errors.New("queries spanning multiple collections are not supported")
	// ErrPoolNotFound is returned when pool was not found by the Router.
	ErrPoolNotFound = errors.New("no hosts serve the requested collection")
	// ErrNoLivePoolMembers is returned when live hosts serve the requested
	// collection but none of them are members of the requested pool.
	ErrNoLivePoolMembers = errors.New("no live hosts in the requested pool")
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

	return r
}

// SelectHosts returns a random sample of up to n hosts that serve the collection,
// are currently live, and are members of the given pool. It intersects the live
// hosts for the collection with the pool's member set, so a query reaches only
// hosts in the window the client signed. Only single-collection queries are
// supported. Returns ErrPoolNotSupported if collections is not exactly one,
// ErrPoolNotFound if no live hosts serve the collection, or ErrNoLivePoolMembers
// if some do but none belong to the pool.
func (r *Router) SelectHosts(_ context.Context, n int, collections []string, members []host.Host) ([]host.Host, error) {
	if len(collections) != 1 {
		return nil, ErrPoolNotSupported
	}

	r.mtx.RLock()
	defer r.mtx.RUnlock()

	p, ok := r.pools[collections[0]]
	if !ok {
		return nil, ErrPoolNotFound
	}

	memberSet := make(map[host.Host]struct{}, len(members))
	for _, m := range members {
		memberSet[m] = struct{}{}
	}

	live := p.hosts.Pool()
	candidates := make([]host.Host, 0, len(live))
	for _, h := range live {
		if _, ok := memberSet[h]; ok {
			candidates = append(candidates, h)
		}
	}
	if len(candidates) == 0 {
		return nil, ErrNoLivePoolMembers
	}

	// ChaCha8 seeded from crypto/rand is a cryptographically strong shuffle source.
	rng := rand.New(rand.NewChaCha8(mustGetSeed())) // nolint:gosec
	rng.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})
	return candidates[:min(n, len(candidates))], nil
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
