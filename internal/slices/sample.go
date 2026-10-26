package slices

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// Sample returns up to n items chosen uniformly at random from items, in
// random order and without repeats. The input slice is not modified. When n is
// not smaller than len(items), every item is returned (shuffled).
func Sample[T any](items []T, n int) []T {
	if n <= 0 || len(items) == 0 {
		return []T{}
	}

	pool := make([]T, len(items))
	copy(pool, items)
	n = min(n, len(pool))

	// Partial Fisher-Yates: settle only the first n positions.
	for i := range n {
		r, err := rand.Int(rand.Reader, big.NewInt(int64(len(pool)-i)))
		if err != nil {
			panic(fmt.Sprintf("failed to generate random index: %v", err))
		}

		j := i + int(r.Int64())
		pool[i], pool[j] = pool[j], pool[i]
	}

	return pool[:n]
}
