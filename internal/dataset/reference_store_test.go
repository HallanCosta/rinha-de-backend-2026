package dataset

import (
	"math"
	"testing"

	"rinha-backend-2026/internal/model"
)

func TestMemoryReferenceStorePacksAndDecodesReferences(t *testing.T) {
	original := model.Reference{
		Vector: model.Vector{0.123456, 0.5, 1, 0, 0.75, -1, -1, 0.25, 0.1, 1, 0, 1, 0.8, 0.9999},
		Label:  model.LabelFraud,
	}
	store, err := NewMemoryReferenceStore([]model.Reference{original})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	var decoded model.Vector
	label, ok := store.At(0, &decoded)
	if !ok || label != model.LabelFraud {
		t.Fatalf("at = (%q, %v), want fraud/true", label, ok)
	}
	for dimension := range original.Vector {
		if original.Vector[dimension] == -1 {
			if decoded[dimension] != -1 {
				t.Errorf("dimension %d = %v, want sentinel -1", dimension, decoded[dimension])
			}
			continue
		}
		if math.Abs(decoded[dimension]-original.Vector[dimension]) > 1.0/packedScale {
			t.Errorf("dimension %d = %.8f, want %.8f", dimension, decoded[dimension], original.Vector[dimension])
		}
	}
}

func TestMemoryReferenceStoreRejectsInvalidVector(t *testing.T) {
	reference := model.Reference{Label: model.LabelLegit}
	reference.Vector[0] = 2
	if _, err := NewMemoryReferenceStore([]model.Reference{reference}); err == nil {
		t.Fatal("new store with invalid vector succeeded, want error")
	}
}
