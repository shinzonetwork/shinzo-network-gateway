package host

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type flapChecker struct{ online *atomic.Bool }

func (f *flapChecker) CheckConnection(_ context.Context, _ Host) ConnectionStatus {
	return ConnectionStatus{Online: f.online.Load()}
}

type fixedCollFetcher struct {
	mtx   sync.Mutex
	colls []string
}

func (f *fixedCollFetcher) FetchCollections(_ context.Context, _ Host) ([]string, error) {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	out := make([]string, len(f.colls))
	copy(out, f.colls)
	return out, nil
}

func (f *fixedCollFetcher) set(c []string) {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	f.colls = append([]string(nil), c...)
}

type recordingObserver struct {
	mtx        sync.Mutex
	addsByHost map[Host][][]string
	rmsByHost  map[Host][][]string
	upCount    map[Host]int
	downCount  map[Host]int
}

func newRecordingObserver() *recordingObserver {
	return &recordingObserver{
		addsByHost: map[Host][][]string{},
		rmsByHost:  map[Host][][]string{},
		upCount:    map[Host]int{},
		downCount:  map[Host]int{},
	}
}

func (r *recordingObserver) Up(h Host)   { r.mtx.Lock(); r.upCount[h]++; r.mtx.Unlock() }
func (r *recordingObserver) Down(h Host) { r.mtx.Lock(); r.downCount[h]++; r.mtx.Unlock() }
func (r *recordingObserver) CollectionsAdded(h Host, c []string) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	cp := append([]string(nil), c...)
	r.addsByHost[h] = append(r.addsByHost[h], cp)
}

func (r *recordingObserver) CollectionsRemoved(h Host, c []string) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	cp := append([]string(nil), c...)
	r.rmsByHost[h] = append(r.rmsByHost[h], cp)
}

func (r *recordingObserver) adds(h Host) [][]string {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	out := make([][]string, len(r.addsByHost[h]))
	copy(out, r.addsByHost[h])
	return out
}

// A host going offline and back online must re-emit its collections so
// observers that cleared state on Down can repopulate.
func TestHostFlapResetsCollections(t *testing.T) {
	t.Parallel()
	logger, _ := zap.NewDevelopment()
	online := &atomic.Bool{}
	online.Store(true)
	cc := &flapChecker{online: online}
	cf := &fixedCollFetcher{colls: []string{"Block"}}
	obs := newRecordingObserver()

	reg := NewRegistry(Config{
		ConnCheckInterval:          50 * time.Millisecond,
		CollectionsRefreshInterval: 50 * time.Millisecond,
	}, nil, []Observer{obs}, cc, cf, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	doneCh := make(chan error, 1)
	go func() { doneCh <- reg.Run(ctx) }()

	h := Host("flap.host")
	reg.register(ctx, h)

	require.Eventually(t, func() bool {
		obs.mtx.Lock()
		defer obs.mtx.Unlock()
		return obs.upCount[h] >= 1 && len(obs.addsByHost[h]) >= 1
	}, 2*time.Second, 5*time.Millisecond, "initial Up+CollectionsAdded not received")

	online.Store(false)
	require.Eventually(t, func() bool {
		obs.mtx.Lock()
		defer obs.mtx.Unlock()
		return obs.downCount[h] >= 1
	}, 2*time.Second, 5*time.Millisecond, "Down not received after host goes offline")

	online.Store(true)
	require.Eventually(t, func() bool {
		obs.mtx.Lock()
		defer obs.mtx.Unlock()
		return obs.upCount[h] >= 2
	}, 2*time.Second, 5*time.Millisecond, "second Up not received after host comes back online")

	// 2s covers ~40 collection-refresh ticks, enough for any late emit to arrive.
	require.Eventually(t, func() bool {
		return len(obs.adds(h)) >= 2
	}, 2*time.Second, 5*time.Millisecond,
		"CollectionsAdded was not re-emitted after host came back online")

	cancel()
	<-doneCh
}

// Registering a host after deregistering it must emit CollectionsAdded
// for the fresh monitor's first collection fetch.
func TestHostDeregisterReregisterDoesEmitCollections(t *testing.T) {
	t.Parallel()
	logger, _ := zap.NewDevelopment()
	online := &atomic.Bool{}
	online.Store(true)
	cc := &flapChecker{online: online}
	cf := &fixedCollFetcher{colls: []string{"Block"}}
	obs := newRecordingObserver()

	reg := NewRegistry(Config{
		ConnCheckInterval:          50 * time.Millisecond,
		CollectionsRefreshInterval: 50 * time.Millisecond,
	}, nil, []Observer{obs}, cc, cf, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	doneCh := make(chan error, 1)
	go func() { doneCh <- reg.Run(ctx) }()

	h := Host("flap.host")
	reg.register(ctx, h)

	require.Eventually(t, func() bool {
		return len(obs.adds(h)) >= 1
	}, 2*time.Second, 5*time.Millisecond, "initial CollectionsAdded not received")

	reg.deregister(ctx, h)

	// Wait for the previous monitor to emit its shutdown notifications.
	require.Eventually(t, func() bool {
		obs.mtx.Lock()
		defer obs.mtx.Unlock()
		return obs.downCount[h] >= 1 && len(obs.rmsByHost[h]) >= 1
	}, 2*time.Second, 5*time.Millisecond, "monitor did not emit shutdown notifications")

	reg.register(ctx, h)
	require.Eventually(t, func() bool {
		return len(obs.adds(h)) >= 2
	}, 2*time.Second, 5*time.Millisecond,
		"deregister+register should emit a second CollectionsAdded")

	cancel()
	<-doneCh
}

// When a host's collection set changes during an offline period, the new
// set must be reported on recovery.
func TestHostFlapWithChangedCollectionsDoesEmit(t *testing.T) {
	t.Parallel()
	logger, _ := zap.NewDevelopment()
	online := &atomic.Bool{}
	online.Store(true)
	cc := &flapChecker{online: online}
	cf := &fixedCollFetcher{colls: []string{"Block"}}
	obs := newRecordingObserver()

	reg := NewRegistry(Config{
		ConnCheckInterval:          50 * time.Millisecond,
		CollectionsRefreshInterval: 50 * time.Millisecond,
	}, nil, []Observer{obs}, cc, cf, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	doneCh := make(chan error, 1)
	go func() { doneCh <- reg.Run(ctx) }()

	h := Host("flap.host")
	reg.register(ctx, h)

	require.Eventually(t, func() bool {
		return len(obs.adds(h)) >= 1
	}, 2*time.Second, 5*time.Millisecond)

	online.Store(false)
	require.Eventually(t, func() bool {
		obs.mtx.Lock()
		defer obs.mtx.Unlock()
		return obs.downCount[h] >= 1
	}, 2*time.Second, 5*time.Millisecond)

	cf.set([]string{"Block", "Tx"})
	online.Store(true)

	require.Eventually(t, func() bool {
		obs.mtx.Lock()
		defer obs.mtx.Unlock()
		return obs.upCount[h] >= 2
	}, 2*time.Second, 5*time.Millisecond)

	// The most recent CollectionsAdded must cover the new set so the
	// Router can populate the pool correctly.
	require.Eventually(t, func() bool {
		adds := obs.adds(h)
		if len(adds) == 0 {
			return false
		}
		last := adds[len(adds)-1]
		var hasBlock, hasTx bool
		for _, c := range last {
			if c == "Block" {
				hasBlock = true
			}
			if c == "Tx" {
				hasTx = true
			}
		}
		return hasBlock && hasTx
	}, 2*time.Second, 5*time.Millisecond,
		"latest CollectionsAdded should cover Block and Tx after recovery")

	cancel()
	<-doneCh
}
