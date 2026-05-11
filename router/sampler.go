package router

import (
	"errors"
	"math/rand/v2"
	"slices"
)

var (
	// ErrNegativeSampleSize is returned by sampler, when requested sample size is negative.
	ErrNegativeSampleSize = errors.New("sample size cannot be negative")

	// ErrSampleExceedsPool is returned by sampler, when requested sample size exceeds pool size.
	ErrSampleExceedsPool = errors.New("sample size exceeds pool size")
)

// Sampler is for sampling n elements from a pool, without replacement.
type Sampler[T any] interface {
	// Sample returns a sample of n elements from the pool, without replacement.
	// If n < 0 or n > pool length, error is returned.
	Sample(n int) ([]T, error)

	// Pool returns all elements in the pool in unspecified order.
	// This method might return direct reference to underlying pool.
	Pool() []T

	// Reset sets the pool and forces re-shuffle on next call to Sample.
	Reset(pool []T)
}

type deckSampler[T any] struct {
	pool []T
	pos  int
	rng  *rand.Rand
}

func newDeckSampler[T any](pool []T, seed [32]byte) *deckSampler[T] {
	return &deckSampler[T]{
		pool: slices.Clone(pool),
		// ChaCha8 is "cryptographically-strong", see: https://go.dev/blog/chacha8rand#the-chacha8rand-generator
		rng: rand.New(rand.NewChaCha8(seed)), // nolint:gosec
	}
}

func (d *deckSampler[T]) Sample(n int) ([]T, error) {
	l := len(d.pool)
	switch {
	case n < 0:
		return nil, ErrNegativeSampleSize
	case n > l:
		return nil, ErrSampleExceedsPool
	case n+d.pos > len(d.pool):
		d.shuffle()
		d.pos = 0
	}

	s := d.pos
	d.pos += n
	return slices.Clone(d.pool[s : s+n]), nil
}

func (d *deckSampler[T]) Pool() []T {
	return d.pool
}

func (d *deckSampler[T]) Reset(pool []T) {
	d.pos = len(pool)
	d.pool = pool
}

func (d *deckSampler[T]) shuffle() {
	d.rng.Shuffle(len(d.pool), func(i, j int) {
		d.pool[i], d.pool[j] = d.pool[j], d.pool[i]
	})
}
