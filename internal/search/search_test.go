package search

import (
	"context"
	"math"
	"testing"

	"rinha-backend-2026/internal/model"
)

func TestBruteForceReturnsOrderedTopK(t *testing.T) {
	store := sliceStore{
		{Vector: vectorAt(0.4), Label: model.LabelLegit},
		{Vector: vectorAt(0.1), Label: model.LabelFraud},
		{Vector: vectorAt(0.6), Label: model.LabelLegit},
		{Vector: vectorAt(0.2), Label: model.LabelFraud},
		{Vector: vectorAt(0.3), Label: model.LabelLegit},
		{Vector: vectorAt(0.5), Label: model.LabelFraud},
	}

	got, err := NewBruteForce(store).TopK(context.Background(), model.Vector{}, 5)
	if err != nil {
		t.Fatalf("top k: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("neighbor count = %d, want 5", len(got))
	}

	wantDistances := []float64{0.1, 0.2, 0.3, 0.4, 0.5}
	wantLabels := []model.Label{model.LabelFraud, model.LabelFraud, model.LabelLegit, model.LabelLegit, model.LabelFraud}
	for index, neighbor := range got {
		if math.Abs(neighbor.Distance-wantDistances[index]) > 1e-12 {
			t.Errorf("distance %d = %.12f, want %.12f", index, neighbor.Distance, wantDistances[index])
		}
		if neighbor.Label != wantLabels[index] {
			t.Errorf("label %d = %q, want %q", index, neighbor.Label, wantLabels[index])
		}
	}
}

func TestBruteForceReturnsAvailableReferencesWhenKIsLarger(t *testing.T) {
	store := sliceStore{{Vector: vectorAt(0.2), Label: model.LabelLegit}}
	got, err := NewBruteForce(store).TopK(context.Background(), model.Vector{}, 5)
	if err != nil {
		t.Fatalf("top k: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("neighbor count = %d, want 1", len(got))
	}
}

func TestBruteForceHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewBruteForce(sliceStore{{Vector: vectorAt(0.1), Label: model.LabelLegit}}).TopK(ctx, model.Vector{}, 1)
	if err != context.Canceled {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestBruteForceRejectsInvalidK(t *testing.T) {
	_, err := NewBruteForce(sliceStore{{Vector: vectorAt(0.1), Label: model.LabelLegit}}).TopK(context.Background(), model.Vector{}, 0)
	if err == nil {
		t.Fatal("top k with zero k succeeded, want error")
	}
}

type sliceStore []model.Reference

func (s sliceStore) Len() int { return len(s) }

func (s sliceStore) At(index int, dst *model.Vector) (model.Label, bool) {
	if index < 0 || index >= len(s) {
		return "", false
	}
	if dst != nil {
		*dst = s[index].Vector
	}
	return s[index].Label, true
}

func vectorAt(value float64) model.Vector {
	vector := model.Vector{}
	vector[0] = value
	return vector
}
