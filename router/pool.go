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

	hosts := p.hosts.Pool()
	if !slices.Contains(hosts, h) {
		hosts = append(hosts, h)
		slices.Sort(hosts)
		hosts = slices.Compact(hosts)

		p.hosts.Reset(hosts)
	}
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
		p.hosts.Reset(hosts)
	}
}

func mustGetSeed() [32]byte {
	var seed [32]byte
	if _, err := rand.Read(seed[:]); err != nil {
		panic(fmt.Errorf("failed while generating random seed: %w", err))
	}

	return seed
}
