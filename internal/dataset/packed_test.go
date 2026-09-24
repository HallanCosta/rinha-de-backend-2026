package dataset

import (
	"math"
	"testing"

	"rinha-backend-2026/internal/model"
)

func TestPackedDistancePreservesSentinelDistance(t *testing.T) {
	left := EncodeVector(model.Vector{0, 0, 0, 0, 0, -1, -1, 0, 0, 0, 0, 0, 0, 0})
	right := EncodeVector(model.Vector{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})

	if got := PackedDistanceSquared(left, right); math.Abs(got-2) > 1e-9 {
		t.Errorf("packed distance squared = %v, want 2", got)
	}
}
