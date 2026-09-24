package search

import (
	"container/heap"
	"context"
	"fmt"
	"math"
	"sort"

	"rinha-backend-2026/internal/dataset"
	"rinha-backend-2026/internal/model"
)

// BruteForce é o baseline exato: percorre todas as referências e mantém
// somente os k melhores em um max-heap. A complexidade é O(N*14*log(k)), sem
// ordenar os milhões de registros completos.
type BruteForce struct {
	references dataset.ReferenceStore
}

// NewBruteForce cria uma busca sobre um store somente-leitura.
func NewBruteForce(references dataset.ReferenceStore) *BruteForce {
	return &BruteForce{references: references}
}

// TopK retorna até k vizinhos em ordem crescente de distância euclidiana.
func (s *BruteForce) TopK(ctx context.Context, query model.Vector, k int) ([]model.Neighbor, error) {
	if s == nil || s.references == nil {
		return nil, fmt.Errorf("reference store is nil")
	}
	if k <= 0 {
		return nil, fmt.Errorf("k must be positive")
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}

	if packed, ok := s.references.(dataset.PackedReferenceStore); ok {
		return s.topKPacked(ctx, query, k, packed)
	}
	if values, ok := s.references.(dataset.ValueReferenceStore); ok {
		return s.topKValues(ctx, query, k, values)
	}
	return s.topKViaOutputParameter(ctx, query, k)
}

func (s *BruteForce) topKPacked(ctx context.Context, query model.Vector, k int, references dataset.PackedReferenceStore) ([]model.Neighbor, error) {
	encodedQuery := dataset.EncodeVector(query)
	candidates := make(candidateHeap, 0, min(k, s.references.Len()))
	heap.Init(&candidates)
	for index := 0; index < s.references.Len(); index++ {
		if index&1023 == 0 {
			if err := contextError(ctx); err != nil {
				return nil, err
			}
		}

		distanceKey, label, ok := references.DistanceKeyAt(index, encodedQuery)
		if !ok {
			return nil, fmt.Errorf("reference store returned no item at index %d", index)
		}
		pushCandidate(&candidates, candidate{
			distanceSquared: float64(distanceKey),
			packed:          true,
			label:           label,
			index:           index,
		}, k)
	}
	return finishCandidates(ctx, candidates)
}

func (s *BruteForce) topKValues(ctx context.Context, query model.Vector, k int, references dataset.ValueReferenceStore) ([]model.Neighbor, error) {
	candidates := make(candidateHeap, 0, min(k, s.references.Len()))
	heap.Init(&candidates)
	for index := 0; index < s.references.Len(); index++ {
		// Checar a cada bloco reduz o custo do contexto no caminho quente, mas
		// ainda permite interromper o scan de um dataset grande rapidamente.
		if index&1023 == 0 {
			if err := contextError(ctx); err != nil {
				return nil, err
			}
		}

		reference, label, ok := references.AtValue(index)
		if !ok {
			return nil, fmt.Errorf("reference store returned no item at index %d", index)
		}
		candidate := candidate{
			distanceSquared: squaredDistance(query, reference),
			label:           label,
			index:           index,
		}

		if len(candidates) < k {
			heap.Push(&candidates, candidate)
			continue
		}
		if candidateLess(candidate, candidates[0]) {
			candidates[0] = candidate
			heap.Fix(&candidates, 0)
		}
	}

	return finishCandidates(ctx, candidates)
}

func (s *BruteForce) topKViaOutputParameter(ctx context.Context, query model.Vector, k int) ([]model.Neighbor, error) {
	candidates := make(candidateHeap, 0, min(k, s.references.Len()))
	heap.Init(&candidates)
	for index := 0; index < s.references.Len(); index++ {
		if index&1023 == 0 {
			if err := contextError(ctx); err != nil {
				return nil, err
			}
		}

		var reference model.Vector
		label, ok := s.references.At(index, &reference)
		if !ok {
			return nil, fmt.Errorf("reference store returned no item at index %d", index)
		}
		candidate := candidate{
			distanceSquared: squaredDistance(query, reference),
			label:           label,
			index:           index,
		}
		pushCandidate(&candidates, candidate, k)
	}
	return finishCandidates(ctx, candidates)
}

func pushCandidate(candidates *candidateHeap, value candidate, k int) {
	if candidates.Len() < k {
		heap.Push(candidates, value)
		return
	}
	if candidateLess(value, (*candidates)[0]) {
		(*candidates)[0] = value
		heap.Fix(candidates, 0)
	}
}

func finishCandidates(ctx context.Context, candidates candidateHeap) ([]model.Neighbor, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	sort.Slice(candidates, func(left, right int) bool {
		return candidateLess(candidates[left], candidates[right])
	})
	result := make([]model.Neighbor, len(candidates))
	for index, item := range candidates {
		distanceSquared := item.distanceSquared
		if item.packed {
			distanceSquared /= dataset.PackedScale * dataset.PackedScale
		}
		result[index] = model.Neighbor{
			Distance: math.Sqrt(distanceSquared),
			Label:    item.label,
		}
	}
	return result, nil
}

type candidate struct {
	distanceSquared float64
	packed          bool
	label           model.Label
	index           int
}

// candidateHeap mantém o pior item no topo para poder substituí-lo em O(log k).
type candidateHeap []candidate

func (h candidateHeap) Len() int { return len(h) }

func (h candidateHeap) Less(left, right int) bool {
	// container/heap trata o menor elemento como topo. Invertendo a comparação,
	// o topo passa a ser o maior (pior) candidato.
	if h[left].distanceSquared != h[right].distanceSquared {
		return h[left].distanceSquared > h[right].distanceSquared
	}
	return h[left].index > h[right].index
}

func (h candidateHeap) Swap(left, right int) { h[left], h[right] = h[right], h[left] }

func (h *candidateHeap) Push(value any) {
	*h = append(*h, value.(candidate))
}

func (h *candidateHeap) Pop() any {
	old := *h
	last := len(old) - 1
	value := old[last]
	*h = old[:last]
	return value
}

func candidateLess(left, right candidate) bool {
	if left.distanceSquared != right.distanceSquared {
		return left.distanceSquared < right.distanceSquared
	}
	return left.index < right.index
}

func squaredDistance(left, right model.Vector) float64 {
	var sum float64
	for dimension := range left {
		difference := left[dimension] - right[dimension]
		sum += difference * difference
	}
	return sum
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
