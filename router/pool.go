package router

import (
	"crypto/rand"
	"fmt"
	"slices"
	"sync"

	"github.com/shinzonetwork/shinzo-network-gateway/host"
	"go.uber.org/zap"
)

type pool struct {
	collection string

	mtx   sync.Mutex
	hosts Sampler[host.Host]

	logger *zap.Logger
}

func newPool(collection string, hosts []host.Host, logger *zap.Logger) *pool {
	return &pool{
		collection: collection,
		hosts:      newDeckSampler(hosts, mustGetSeed()),
		logger:     logger.Named("pool:" + collection),
	}
}

func (p *pool) get(n int) ([]host.Host, error) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	return p.hosts.Sample(n)
}

func (p *pool) add(h host.Host) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	// TODO(tzdybal): consider checking if host is already in the pool
	hosts := append(p.hosts.Pool(), h)
	slices.Sort(hosts)
	hosts = slices.Compact(hosts)

	p.hosts = newDeckSampler(hosts, mustGetSeed())
}

func (p *pool) remove(h host.Host) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	hosts := p.hosts.Pool()
	n := len(hosts)
	hosts = slices.DeleteFunc(p.hosts.Pool(), func(hh host.Host) bool {
		return hh == h
	})

	if len(hosts) != n {
		p.hosts = newDeckSampler(hosts, mustGetSeed())
	}
}

func mustGetSeed() [32]byte {
	var seed [32]byte
	if _, err := rand.Read(seed[:]); err != nil {
		panic(fmt.Errorf("failed while generating random seed: %w", err))
	}

	return seed
}
