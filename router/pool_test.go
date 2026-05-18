package router

import (
	"sync"
	"testing"

	"github.com/shinzonetwork/shinzo-network-gateway/host"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestPool(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		initial  []host.Host
		add      []host.Host
		remove   []host.Host
		expected []host.Host
		expErr   error
	}{
		{
			name:     "simple add",
			initial:  []host.Host{"host1", "host2"},
			add:      []host.Host{"host3"},
			expected: []host.Host{"host1", "host2", "host3"},
		},
		{
			name:     "add many",
			initial:  []host.Host{"host1", "host2"},
			add:      []host.Host{"host3", "host4"},
			expected: []host.Host{"host1", "host2", "host3", "host4"},
		},
		{
			name:     "simple remove",
			initial:  []host.Host{"host1", "host2", "host3"},
			remove:   []host.Host{"host2"},
			expected: []host.Host{"host1", "host3"},
		},
		{
			name:     "add with duplicate",
			initial:  []host.Host{"host1", "host2", "host3"},
			add:      []host.Host{"host2"},
			expected: []host.Host{"host1", "host2", "host3"},
		},
		{
			name:     "add with and without duplicate",
			initial:  []host.Host{"host1", "host2", "host3"},
			add:      []host.Host{"host2", "host4"},
			expected: []host.Host{"host1", "host2", "host3", "host4"},
		},
		{
			name:     "remove non-existent",
			initial:  []host.Host{"host1", "host2", "host3"},
			remove:   []host.Host{"host4"},
			expected: []host.Host{"host1", "host2", "host3"},
		},
		{
			name:     "add and remove",
			initial:  []host.Host{"host1", "host2", "host3"},
			add:      []host.Host{"host4"},
			remove:   []host.Host{"host2"},
			expected: []host.Host{"host1", "host3", "host4"},
		},
		{
			name:     "remove added",
			initial:  []host.Host{"host1", "host2", "host3"},
			add:      []host.Host{"host4"},
			remove:   []host.Host{"host4"},
			expected: []host.Host{"host1", "host2", "host3"},
		},
		{
			name:     "remove many",
			initial:  []host.Host{"host1", "host2", "host3"},
			remove:   []host.Host{"host1", "host2", "host4"},
			expected: []host.Host{"host3"},
		},
		{
			name:    "remove all",
			initial: []host.Host{"host1", "host2", "host3"},
			remove:  []host.Host{"host1", "host2", "host3"},
			expErr:  ErrNoHostsAvailable,
		},
		{
			name:     "start empty",
			initial:  []host.Host{},
			add:      []host.Host{"host1", "host2", "host3"},
			expected: []host.Host{"host1", "host2", "host3"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			logger := zap.NewNop()
			pool := newPool(c.name, c.initial, logger)
			require.NotNil(t, pool)

			if len(c.initial) > 0 {
				// force randomization
				_, err := pool.get(1)
				require.NoError(t, err)
			}

			for _, h := range c.add {
				pool.add(h)
			}
			for _, h := range c.remove {
				pool.remove(h)
			}

			actual, err := pool.get(len(c.expected))
			if c.expErr != nil {
				require.ErrorIs(t, err, ErrNoHostsAvailable)
				require.Nil(t, actual)
			} else {
				require.NoError(t, err)
				require.ElementsMatch(t, c.expected, actual)
			}
			require.Len(t, pool.hosts.Pool(), len(c.expected))
		})
	}
}

func TestPoolParallel(t *testing.T) {
	t.Parallel()

	const reps = 100

	// to be able to assert expected slice, add and remove have to be disjoint
	cases := []struct {
		name     string
		initial  []host.Host
		add      []host.Host
		remove   []host.Host
		expected []host.Host
	}{
		{
			name:     "only add",
			initial:  []host.Host{"host1", "host2"},
			add:      []host.Host{"host3", "host4"},
			expected: []host.Host{"host1", "host2", "host3", "host4"},
		},
		{
			name:     "remove hard",
			initial:  []host.Host{"host1", "host2", "host3", "host4"},
			remove:   []host.Host{"host3", "host4"},
			expected: []host.Host{"host1", "host2"},
		},
		{
			name:     "add and remove",
			initial:  []host.Host{"host1", "host2", "host3", "host4"},
			add:      []host.Host{"host5", "host6"},
			remove:   []host.Host{"host3", "host4"},
			expected: []host.Host{"host1", "host2", "host5", "host6"},
		},
		{
			name:    "add and remove same",
			initial: []host.Host{"host1", "host2", "host3", "host4"},
			add:     []host.Host{"host4", "host5", "host6"},
			remove:  []host.Host{"host4", "host5", "host6"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			logger := zap.NewNop()
			pool := newPool(c.name, c.initial, logger)
			require.NotNil(t, pool)

			wg := sync.WaitGroup{}
			wg.Go(func() {
				// force randomization
				for range 5 * reps {
					_, err := pool.get(1)
					require.NoError(t, err)
				}
			})
			wg.Go(func() {
				for range reps {
					for _, h := range c.add {
						pool.add(h)
					}
				}
			})
			wg.Go(func() {
				for range reps {
					for _, h := range c.remove {
						pool.remove(h)
					}
				}
			})

			wg.Wait()

			if len(c.expected) > 0 {
				actual, err := pool.get(len(c.expected))
				require.NoError(t, err)
				require.ElementsMatch(t, c.expected, actual)
				require.Len(t, pool.hosts.Pool(), len(c.expected))
			}
		})
	}
}
