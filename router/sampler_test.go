package router

import (
	"crypto/rand"
	"math"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func mustGetDeckSampler(t *testing.T, n int) *deckSampler[string] {
	t.Helper()
	var seed [32]byte
	_, err := rand.Read(seed[:])
	require.NoError(t, err)

	pool := make([]string, n)
	for i := range n {
		pool[i] = strconv.Itoa(i)
	}

	sampler := newDeckSampler(pool, seed)
	require.NotNil(t, sampler)
	return sampler
}

func TestDeckSamplerEdges(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		n    int
		s    int
		err  error
	}{
		{
			name: "single item",
			n:    10,
			s:    1,
			err:  nil,
		},
		{
			// it makes not much sense, but sampling 0 elements is not invalid
			name: "zero items",
			n:    10,
			s:    0,
			err:  nil,
		},
		{
			name: "negative items",
			n:    10,
			s:    -1,
			err:  ErrNegativeSampleSize,
		},
		{
			name: "exactly all items",
			n:    10,
			s:    10,
			err:  nil,
		},
		{
			name: "too many items",
			n:    10,
			s:    11,
			err:  ErrSampleExceedsPool,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			sampler := mustGetDeckSampler(t, c.n)
			sample, err := sampler.Sample(c.s)
			if c.err == nil {
				require.NoError(t, err)
				require.Len(t, sample, c.s)
			} else {
				require.ErrorIs(t, err, c.err)
				require.Nil(t, sample)
			}
		})
	}
}

func TestDeckSamplerReshuffle(t *testing.T) {
	t.Parallel()

	const (
		n = 10
		s = 3
	)
	sampler := mustGetDeckSampler(t, n)

	// as long as pool/deck is not exhausted, we must peek unique items
	k := n / s
	sampled := make([]string, 0, k*s)
	for range k {
		sample, err := sampler.Sample(s)
		require.NoError(t, err)
		require.Len(t, sample, s)
		require.Subset(t, sampler.pool, sample)
		sampled = append(sampled, sample...)
	}
	// sort + compact to ensure that there are no duplicates
	slices.Sort(sampled)
	require.Len(t, slices.Compact(sampled), k*s)

	// run some more sampling to ensure it's not crashing
	for range n {
		sample, err := sampler.Sample(s)
		require.NoError(t, err)
		require.Len(t, sample, s)
		require.Subset(t, sampler.pool, sample)
	}
}

func TestDeckSamplerFairness(t *testing.T) {
	t.Parallel()

	const (
		n           = 100
		s           = 15
		cycles      = 5000
		stddevLimit = 5.0 // to avoid test flakiness, allow some deviation factor
	)

	// Per cycle the deck draws (n/s)*s distinct items uniformly from the pool,
	// so each item's total count is Binomial(cycles, p) with p = drawn/pool.
	p := float64((n/s)*s) / float64(n)
	mean := p * float64(cycles)
	stddev := math.Sqrt(float64(cycles) * p * (1 - p))

	sampler := mustGetDeckSampler(t, n)
	cnt := make(map[string]int)
	for range cycles * (n / s) {
		sample, err := sampler.Sample(s)
		require.NoError(t, err)
		for _, item := range sample {
			cnt[item]++
		}
	}
	require.Len(t, cnt, n)

	for item, c := range cnt {
		require.Lessf(t, math.Abs(float64(c)-mean)/stddev, stddevLimit,
			"item %s drawn %d times, expected %.1f ± %.1f, stddevLimit=%.1f",
			item, c, mean, stddev, stddevLimit)
	}
}
