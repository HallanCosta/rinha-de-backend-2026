package search

import (
	"context"
	"path/filepath"
	"testing"

	"rinha-backend-2026/internal/dataset"
	"rinha-backend-2026/internal/model"
)

func BenchmarkBruteForceTopKOfficial(b *testing.B) {
	store, err := dataset.LoadReferences(context.Background(), filepath.Join("..", "..", "resources", "references.json.gz"))
	if err != nil {
		b.Fatalf("load official references: %v", err)
	}
	searcher := NewBruteForce(store)
	query := model.Vector{0.0041, 0.1667, 0.05, 0.7826, 0.3333, -1, -1, 0.0292, 0.15, 0, 1, 0, 0.15, 0.006}

	b.ReportMetric(float64(store.Len()), "references")
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := searcher.TopK(context.Background(), query, 5); err != nil {
			b.Fatalf("top k: %v", err)
		}
	}
}

func BenchmarkBruteForceTopKOfficialParallel(b *testing.B) {
	store, err := dataset.LoadReferences(context.Background(), filepath.Join("..", "..", "resources", "references.json.gz"))
	if err != nil {
		b.Fatalf("load official references: %v", err)
	}
	searcher := NewBruteForce(store)
	query := model.Vector{0.0041, 0.1667, 0.05, 0.7826, 0.3333, -1, -1, 0.0292, 0.15, 0, 1, 0, 0.15, 0.006}

	b.ReportMetric(float64(store.Len()), "references")
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pointer *testing.PB) {
		for pointer.Next() {
			if _, err := searcher.TopK(context.Background(), query, 5); err != nil {
				b.Errorf("top k: %v", err)
			}
		}
	})
}
