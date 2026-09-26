//go:build goexperiment.simd

package dataset

import (
	"math/rand"
	"testing"
)

func TestPackedDistanceSIMDMatchesScalar(t *testing.T) {
	random := rand.New(rand.NewSource(42))
	for iteration := 0; iteration < 1000; iteration++ {
		var left, right PackedVector
		for dimension := range left {
			left[dimension] = uint16(random.Intn(int(packedScale) + 1))
			right[dimension] = uint16(random.Intn(int(packedScale) + 1))
		}
		if iteration%2 == 0 {
			left[5] = packedSentinel
		}
		if iteration%3 == 0 {
			right[6] = packedSentinel
		}

		if got, want := packedDistanceKey(left, right), packedDistanceKeyScalarForTest(left, right); got != want {
			t.Fatalf("iteration %d: SIMD distance=%d, scalar distance=%d", iteration, got, want)
		}
	}
}

func packedDistanceKeyScalarForTest(left, right PackedVector) uint64 {
	var sum uint64
	for dimension := range left {
		difference := packedDifferenceKey(left[dimension], right[dimension])
		sum += uint64(difference * difference)
	}
	return sum
}
