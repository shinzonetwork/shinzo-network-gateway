package host

import (
	"context"
	"maps"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const defaultInterval = 5 * time.Second

var defaultConfig = Config{
	ConnCheckInterval:          defaultInterval,
	CollectionsRefreshInterval: defaultInterval,
}

type mockObserver struct {
	mtx   sync.Mutex
	hosts map[Host]struct{}
}

var _ Observer = &mockObserver{}

func newMockObserver() *mockObserver {
	return &mockObserver{
		hosts: make(map[Host]struct{}),
	}
}

func (m *mockObserver) Up(h Host) {
	m.mtx.Lock()
	m.hosts[h] = struct{}{}
	m.mtx.Unlock()
}

func (m *mockObserver) Down(h Host) {
	m.mtx.Lock()
	delete(m.hosts, h)
	m.mtx.Unlock()
}

func (m *mockObserver) CollectionsAdded(_ Host, _ []string) {
	// noop for now
}

func (m *mockObserver) CollectionsRemoved(_ Host, _ []string) {
	// noop for now
}

func (m *mockObserver) hostList() []Host {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	return slices.Collect(maps.Keys(m.hosts))
}

type mockConnChecker struct{}

func (*mockConnChecker) CheckConnection(_ context.Context, _ Host) ConnectionStatus {
	return ConnectionStatus{Online: true}
}

type mockCollFetcher struct{}

func (*mockCollFetcher) FetchCollections(_ context.Context, _ Host) ([]string, error) {
	return nil, nil
}

func TestNewRegistry(t *testing.T) {
	t.Parallel()
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	providers := make([]Provider, 10)
	reg := NewRegistry(defaultConfig, providers, nil, nil, nil, logger)
	require.NotNil(t, reg)
	require.NotEmpty(t, reg.providers)
}

func TestRegistryStartStop(t *testing.T) {
	t.Parallel()
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	defer func() {
		_ = logger.Sync()
	}()

	providers := []Provider{
		NewMockProvider([]Host{"a.b.c", "127.0.0.1"}),
		NewMockProvider([]Host{"x.y.z", "192.168.0.1"}),
		NewMockProvider([]Host{"shinzo.network", "127.0.0.1"}),
	}
	for _, provider := range providers {
		provider.SetLogger(logger)
	}

	observer := newMockObserver()

	reg := NewRegistry(defaultConfig, providers, []Observer{observer}, &mockConnChecker{}, &mockCollFetcher{}, logger)
	require.NotNil(t, reg)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- reg.Run(ctx)
	}()

	require.Eventually(t, func() bool {
		return len(observer.hostList()) == 5
	}, 500*time.Millisecond, 50*time.Millisecond)
	cancel()

	err = <-errCh
	require.NoError(t, err)
}

func TestDeregisterUnknownHost(t *testing.T) {
	t.Parallel()
	logger, _ := zap.NewDevelopment()
	reg := NewRegistry(defaultConfig, nil, nil, &mockConnChecker{}, &mockCollFetcher{}, logger)
	require.NotNil(t, reg)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	require.NotPanics(t, func() { reg.deregister(ctx, Host("never.registered")) })
}

func TestDuplicateHostRegistration(t *testing.T) {
	t.Parallel()
	logger, _ := zap.NewDevelopment()
	observer := newMockObserver()
	// both providers advertise the same host
	providers := []Provider{
		NewMockProvider([]Host{"duplicate.host"}),
		NewMockProvider([]Host{"duplicate.host"}),
	}
	for _, p := range providers {
		p.SetLogger(logger)
	}

	reg := NewRegistry(defaultConfig, providers, []Observer{observer}, &mockConnChecker{}, &mockCollFetcher{}, logger)
	require.NotNil(t, reg)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go func() {
		err := reg.Run(ctx)
		require.NoError(t, err)
	}()

	require.Eventually(t, func() bool {
		reg.mtx.Lock()
		defer reg.mtx.Unlock()
		return len(reg.monitors) == 1 // dedup: only one monitor
	}, 300*time.Millisecond, 10*time.Millisecond)

	require.Len(t, reg.monitors, 1) // make sure it's still 1
}
