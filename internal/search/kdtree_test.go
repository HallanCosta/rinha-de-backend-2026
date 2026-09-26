package search

import (
	"context"
	"math"
	"testing"

	"rinha-backend-2026/internal/dataset"
	"rinha-backend-2026/internal/model"
)

func TestKDTreeMatchesBruteForce(t *testing.T) {
	references := make([]model.Reference, 0, 128)
	for index := 0; index < 128; index++ {
		vector := model.Vector{}
		for dimension := range vector {
			if (index+dimension)%17 == 0 && (dimension == 5 || dimension == 6) {
				vector[dimension] = -1
				continue
			}
			vector[dimension] = float64((index*dimension+dimension*3)%101) / 100
		}
		label := model.LabelLegit
		if index%5 == 0 {
			label = model.LabelFraud
		}
		references = append(references, model.Reference{Vector: vector, Label: label})
	}

	store, err := dataset.NewMemoryReferenceStore(references)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	tree, err := NewKDTree(store)
	if err != nil {
		t.Fatalf("create kd-tree: %v", err)
	}
	brute := NewBruteForce(store)

	for queryIndex := 0; queryIndex < 20; queryIndex++ {
		query := model.Vector{}
		for dimension := range query {
			if (queryIndex+dimension)%19 == 0 && (dimension == 5 || dimension == 6) {
				query[dimension] = -1
				continue
			}
			query[dimension] = float64((queryIndex*dimension+7)%101) / 100
		}

		want, err := brute.TopK(context.Background(), query, 5)
		if err != nil {
			t.Fatalf("brute force query %d: %v", queryIndex, err)
		}
		got, err := tree.TopK(context.Background(), query, 5)
		if err != nil {
			t.Fatalf("kd-tree query %d: %v", queryIndex, err)
		}
		assertNeighborsEqual(t, got, want)
	}
}

func TestKDTreeUsesOriginalIndexToBreakDistanceTies(t *testing.T) {
	references := []model.Reference{
		{Vector: vectorAt(0.1), Label: model.LabelFraud},
		{Vector: vectorAt(0.1), Label: model.LabelLegit},
		{Vector: vectorAt(0.8), Label: model.LabelLegit},
	}
	store, err := dataset.NewMemoryReferenceStore(references)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	tree, err := NewKDTree(store)
	if err != nil {
		t.Fatalf("create kd-tree: %v", err)
	}

	got, err := tree.TopK(context.Background(), model.Vector{}, 1)
	if err != nil {
		t.Fatalf("top k: %v", err)
	}
	if len(got) != 1 || got[0].Label != model.LabelFraud {
		t.Fatalf("neighbors = %#v, want first reference with fraud label", got)
	}
}

func TestKDTreeReturnsAvailableReferencesWhenKIsLarger(t *testing.T) {
	store, err := dataset.NewMemoryReferenceStore([]model.Reference{
		{Vector: vectorAt(0.2), Label: model.LabelLegit},
	})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	tree, err := NewKDTree(store)
	if err != nil {
		t.Fatalf("create kd-tree: %v", err)
	}

	got, err := tree.TopK(context.Background(), model.Vector{}, 5)
	if err != nil {
		t.Fatalf("top k: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("neighbor count = %d, want 1", len(got))
	}
}

func TestKDTreeHonorsCancellationAndInvalidK(t *testing.T) {
	store, err := dataset.NewMemoryReferenceStore([]model.Reference{
		{Vector: vectorAt(0.2), Label: model.LabelLegit},
	})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	tree, err := NewKDTree(store)
	if err != nil {
		t.Fatalf("create kd-tree: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := tree.TopK(ctx, model.Vector{}, 1); err != context.Canceled {
		t.Fatalf("canceled query error = %v, want context.Canceled", err)
	}
	if _, err := tree.TopK(context.Background(), model.Vector{}, 0); err == nil {
		t.Fatal("zero k query succeeded, want error")
	}
}

func assertNeighborsEqual(t *testing.T, got, want []model.Neighbor) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("neighbor count = %d, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index].Label != want[index].Label {
			t.Errorf("neighbor %d label = %q, want %q", index, got[index].Label, want[index].Label)
		}
		if math.Abs(got[index].Distance-want[index].Distance) > 1e-12 {
			t.Errorf("neighbor %d distance = %.12f, want %.12f", index, got[index].Distance, want[index].Distance)
		}
	}
}
