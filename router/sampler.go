package router

import (
	"errors"
	"math/rand/v2"
	"slices"
)

// ErrSampleExceedsPool is returned by sampler, when requested sample size exceeds pool size.
var ErrSampleExceedsPool = errors.New("sample size exceeds pool size")

// Sampler is for sampling n elements from a pool, without replacement.
type Sampler[T any] interface {
	Sample(n int) ([]T, error)
}

type deckSampler[T any] struct {
	pool []T
	pos  int
	rng  *rand.Rand
}

func newDeckSampler[T any](pool []T, seed [32]byte) *deckSampler[T] {
	return &deckSampler[T]{
		pool: pool,
		// ChaCha8 is "cryptographically-strong", see: https://go.dev/blog/chacha8rand#the-chacha8rand-generator
		rng: rand.New(rand.NewChaCha8(seed)), // nolint:gosec
	}
}

func (d *deckSampler[T]) Sample(n int) ([]T, error) {
	l := len(d.pool)
	if n > l {
		return nil, ErrSampleExceedsPool
	}
	if n+d.pos > len(d.pool) {
		d.shuffle()
		d.pos = 0
	}

	s := d.pos
	d.pos += n
	return slices.Clone(d.pool[s : s+n]), nil
}

func (d *deckSampler[T]) shuffle() {
	d.rng.Shuffle(len(d.pool), func(i, j int) {
		d.pool[i], d.pool[j] = d.pool[j], d.pool[i]
	})
}
