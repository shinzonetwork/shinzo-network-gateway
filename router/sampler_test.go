package router

import (
	"crypto/rand"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"gonum.org/v1/gonum/stat/distuv"
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

func TestDeckSamplerCorrectness(t *testing.T) {
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

func TestDeckSamplerDistribution(t *testing.T) {
	t.Parallel()

	const (
		n     = 100
		s     = 15
		reps  = (n/s + 1) * 100
		alpha = float64(0.05)
	)

	sampler := mustGetDeckSampler(t, n)

	cnt := make(map[string]int)
	for range reps {
		sample, err := sampler.Sample(s)
		require.NoError(t, err)
		require.Len(t, sample, s)
		for _, item := range sample {
			cnt[item]++
		}
	}
	require.Len(t, cnt, n)

	chi2 := chi2(cnt)
	p := distuv.ChiSquared{K: float64(n - 1)}.Survival(chi2)

	require.Greaterf(t, p, alpha, "sample distribution is not uniform after %d repetitions, chi squared: %f, p: %f", reps, chi2, p)
}

func chi2(histogram map[string]int) float64 {
	k := len(histogram)
	if k < 2 {
		return 0
	}

	sum := 0
	for _, c := range histogram {
		sum += c
	}

	expected := float64(sum) / float64(k)
	chi2 := float64(0)
	for _, c := range histogram {
		d := float64(c) - expected
		chi2 += d * d / expected
	}

	return chi2
}
