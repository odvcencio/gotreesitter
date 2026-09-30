//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"flag"
	"hash/fnv"
	"math/rand"
	"strconv"
)

// Go's test shuffle flag orders top-level benchmarks. These workloads contain
// subbenchmarks, so apply its explicit seed to those groups as well.
func verifyCostBenchOrder(name string, count int) []int {
	order := make([]int, count)
	for i := range order {
		order[i] = i
	}
	shuffle := flag.Lookup("test.shuffle")
	if shuffle == nil {
		return order
	}
	seed, err := strconv.ParseInt(shuffle.Value.String(), 10, 64)
	if err != nil {
		return order
	}
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(name))
	rng := rand.New(rand.NewSource(seed ^ int64(hash.Sum64())))
	rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	return order
}
