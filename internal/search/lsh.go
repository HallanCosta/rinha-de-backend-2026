package search

import (
	"container/heap"
	"context"
	"fmt"
	"sort"

	"rinha-backend-2026/internal/dataset"
	"rinha-backend-2026/internal/model"
)

const (
	lshBits           = 20
	lshGridBits       = 3
	lshGridDimensions = 4
	lshProbeRadius    = 0
	lshMaxCandidates  = 512
)

// LSH é um índice ANN baseado em hiperplanos aleatórios. Cada referência cai
// em um bucket de 64 bits; a consulta verifica o bucket exato ou uma pequena
// vizinhança de diferenças de Hamming. Isso troca a garantia exata da KD-tree por
// um número previsível de candidatos, opção permitida pela documentação.
type LSH struct {
	references       dataset.ReferenceStore
	distanceStore    packedDistanceStore
	planes           [lshBits][model.VectorDimensions]int8
	alternatePlanes  [lshBits][model.VectorDimensions]int8
	entries          []lshEntry
	alternateEntries []lshEntry
	coarseEntries    []lshCoarseEntry
	probes           []uint64
	fallbackProbes   []uint64
	maxCandidates    int
}

type lshEntry struct {
	signature  uint64
	projection int32
	index      uint32
}

type lshCoarseEntry struct {
	signature  uint32
	projection int32
	index      uint32
}

// NewLSH constrói o índice consultando o bucket da assinatura exata. A
// construção acontece no startup e não no caminho HTTP.
func NewLSH(references dataset.ReferenceStore) (*LSH, error) {
	return NewLSHWithCandidateLimit(references, lshProbeRadius, lshMaxCandidates)
}

// NewLSHWithProbeRadius permite testar o equilíbrio entre recall e custo. Uma
// distância zero lê um bucket; uma distância maior amplia a vizinhança de
// Hamming, sem duplicar uma referência porque cada item pertence a um bucket.
func NewLSHWithProbeRadius(references dataset.ReferenceStore, radius int) (*LSH, error) {
	return NewLSHWithCandidateLimit(references, radius, lshMaxCandidates)
}

// NewLSHWithCandidateLimit existe para os benchmarks de qualidade e permite
// testar a janela máxima sem duplicar a construção do índice.
func NewLSHWithCandidateLimit(references dataset.ReferenceStore, radius, maxCandidates int) (*LSH, error) {
	if references == nil {
		return nil, fmt.Errorf("reference store is nil")
	}
	if radius < 0 || radius > lshBits {
		return nil, fmt.Errorf("LSH probe radius must be between 0 and %d", lshBits)
	}
	if maxCandidates < 0 {
		return nil, fmt.Errorf("LSH candidate limit must not be negative")
	}

	index := &LSH{
		references:       references,
		distanceStore:    nil,
		planes:           newLSHPlanes(0x9e3779b9),
		alternatePlanes:  newLSHPlanes(0x243f6a88),
		entries:          make([]lshEntry, references.Len()),
		alternateEntries: make([]lshEntry, references.Len()),
		coarseEntries:    make([]lshCoarseEntry, references.Len()),
		probes:           lshProbeMasks(radius),
		maxCandidates:    maxCandidates,
	}
	if radius == 0 {
		expandedProbes := lshProbeMasks(3)
		index.fallbackProbes = expandedProbes[1:]
	}
	index.distanceStore, _ = references.(packedDistanceStore)

	for item := range index.entries {
		vector, _, ok := packedAt(references, item)
		if !ok {
			return nil, fmt.Errorf("reference store returned no item at index %d", item)
		}
		signature := lshSignature(vector, &index.planes)
		alternateSignature := lshSignature(vector, &index.alternatePlanes)
		index.entries[item] = lshEntry{
			signature:  signature,
			projection: lshProjection(vector),
			index:      uint32(item),
		}
		index.alternateEntries[item] = lshEntry{
			signature:  alternateSignature,
			projection: index.entries[item].projection,
			index:      uint32(item),
		}
		index.coarseEntries[item] = lshCoarseEntry{
			signature:  lshCoarseSignature(vector),
			projection: index.entries[item].projection,
			index:      uint32(item),
		}
	}

	sortLSHEntries(index.entries)
	sortLSHEntries(index.alternateEntries)
	sort.Slice(index.coarseEntries, func(left, right int) bool {
		if index.coarseEntries[left].signature != index.coarseEntries[right].signature {
			return index.coarseEntries[left].signature < index.coarseEntries[right].signature
		}
		if index.coarseEntries[left].projection != index.coarseEntries[right].projection {
			return index.coarseEntries[left].projection < index.coarseEntries[right].projection
		}
		return index.coarseEntries[left].index < index.coarseEntries[right].index
	})
	return index, nil
}

// TopK retorna os candidatos mais próximos entre os buckets sondados. O
// cálculo da distância continua exato para cada candidato; a aproximação está
// somente na escolha dos buckets visitados.
func (s *LSH) TopK(ctx context.Context, query model.Vector, k int) ([]model.Neighbor, error) {
	if s == nil || s.references == nil {
		return nil, fmt.Errorf("LSH reference store is nil")
	}
	if k <= 0 {
		return nil, fmt.Errorf("k must be positive")
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}

	queryPacked := dataset.EncodeVector(query)
	signature := lshSignature(queryPacked, &s.planes)
	alternateSignature := lshSignature(queryPacked, &s.alternatePlanes)
	projection := lshProjection(queryPacked)
	candidates := make(candidateHeap, 0, k)
	heap.Init(&candidates)
	if _, err := s.scanProbes(ctx, queryPacked, signature, projection, s.entries, s.probes, k, &candidates, false); err != nil {
		return nil, err
	}
	if candidates.Len() < k {
		if _, err := s.scanProbes(ctx, queryPacked, alternateSignature, projection, s.alternateEntries, s.probes, k, &candidates, false); err != nil {
			return nil, err
		}
	}
	// Buckets esparsos ampliam a vizinhança somente quando necessário. Isso
	// evita o fallback O(N) que destruiria o p99 justamente nas consultas raras.
	if candidates.Len() < k {
		if _, err := s.scanProbes(ctx, queryPacked, signature, projection, s.entries, s.fallbackProbes, k, &candidates, true); err != nil {
			return nil, err
		}
	}
	if candidates.Len() < k {
		if _, err := s.scanProbes(ctx, queryPacked, alternateSignature, projection, s.alternateEntries, s.fallbackProbes, k, &candidates, true); err != nil {
			return nil, err
		}
	}
	if candidates.Len() < k {
		coarseSignature := lshCoarseSignature(queryPacked)
		start, end := s.rangeForCoarseQuery(coarseSignature, projection)
		for position := start; position < end; position++ {
			index := int(s.coarseEntries[position].index)
			distanceKey, label, ok := s.distanceAt(index, queryPacked)
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
	}
	return finishCandidates(ctx, candidates)
}

func (s *LSH) scanProbes(
	ctx context.Context,
	query dataset.PackedVector,
	signature uint64,
	projection int32,
	entries []lshEntry,
	probes []uint64,
	k int,
	candidates *candidateHeap,
	stopAtK bool,
) (int, error) {
	scanned := 0
	for _, mask := range probes {
		start, end := rangeForEntries(entries, signature^mask, projection, s.maxCandidates)
		for position := start; position < end; position++ {
			if s.maxCandidates > 0 && scanned >= s.maxCandidates {
				return scanned, nil
			}
			index := int(entries[position].index)
			distanceKey, label, ok := s.distanceAt(index, query)
			if !ok {
				return scanned, fmt.Errorf("reference store returned no item at index %d", index)
			}
			scanned++
			pushCandidate(candidates, candidate{
				distanceSquared: float64(distanceKey),
				packed:          true,
				label:           label,
				index:           index,
			}, k)
		}
		if stopAtK && candidates.Len() >= 5 {
			break
		}
		if err := contextError(ctx); err != nil {
			return scanned, err
		}
	}
	return scanned, nil
}

func (s *LSH) rangeForSignature(signature uint64) (int, int) {
	return rangeForEntries(s.entries, signature, 0, 0)
}

func (s *LSH) rangeForQuery(signature uint64, projection int32) (int, int) {
	return rangeForEntries(s.entries, signature, projection, s.maxCandidates)
}

func rangeForEntries(entries []lshEntry, signature uint64, projection int32, maxCandidates int) (int, int) {
	start := sort.Search(len(entries), func(index int) bool {
		return entries[index].signature >= signature
	})
	end := start + sort.Search(len(entries)-start, func(index int) bool {
		return entries[start+index].signature > signature
	})
	if maxCandidates == 0 || end-start <= maxCandidates {
		return start, end
	}

	position := start + sort.Search(end-start, func(index int) bool {
		return entries[start+index].projection >= projection
	})
	half := maxCandidates / 2
	windowStart := position - half
	if windowStart < start {
		windowStart = start
	}
	windowEnd := windowStart + maxCandidates
	if windowEnd > end {
		windowEnd = end
		windowStart = windowEnd - maxCandidates
		if windowStart < start {
			windowStart = start
		}
	}
	return windowStart, windowEnd
}

func (s *LSH) rangeForCoarseQuery(signature uint32, projection int32) (int, int) {
	start := sort.Search(len(s.coarseEntries), func(index int) bool {
		return s.coarseEntries[index].signature >= signature
	})
	end := start + sort.Search(len(s.coarseEntries)-start, func(index int) bool {
		return s.coarseEntries[start+index].signature > signature
	})
	if s.maxCandidates == 0 || end-start <= s.maxCandidates {
		return start, end
	}

	position := start + sort.Search(end-start, func(index int) bool {
		return s.coarseEntries[start+index].projection >= projection
	})
	half := s.maxCandidates / 2
	windowStart := position - half
	if windowStart < start {
		windowStart = start
	}
	windowEnd := windowStart + s.maxCandidates
	if windowEnd > end {
		windowEnd = end
		windowStart = windowEnd - s.maxCandidates
		if windowStart < start {
			windowStart = start
		}
	}
	return windowStart, windowEnd
}

func (s *LSH) distanceAt(index int, query dataset.PackedVector) (uint64, model.Label, bool) {
	if s.distanceStore != nil {
		return s.distanceStore.DistanceKeyAt(index, query)
	}
	vector, label, ok := packedAt(s.references, index)
	if !ok {
		return 0, "", false
	}
	return dataset.PackedDistanceKey(query, vector), label, true
}

func sortLSHEntries(entries []lshEntry) {
	sort.Slice(entries, func(left, right int) bool {
		if entries[left].signature != entries[right].signature {
			return entries[left].signature < entries[right].signature
		}
		if entries[left].projection != entries[right].projection {
			return entries[left].projection < entries[right].projection
		}
		return entries[left].index < entries[right].index
	})
}

func newLSHPlanes(seed uint32) [lshBits][model.VectorDimensions]int8 {
	var planes [lshBits][model.VectorDimensions]int8
	for bit := range planes {
		for dimension := range planes[bit] {
			seed = seed*1664525 + 1013904223
			// Os bits baixos de um LCG alternam com pouca aleatoriedade. Usar
			// o bit alto evita que os hiperplanos caiam em padrões repetidos.
			if seed&0x80000000 == 0 {
				planes[bit][dimension] = -1
			} else {
				planes[bit][dimension] = 1
			}
		}
	}
	return planes
}

func lshProjection(vector dataset.PackedVector) int32 {
	weights := [...]int32{1, -3, 5, -7, 11, -13, 17, -19, 23, -29, 31, -37, 41, -43}
	var projection int32
	for dimension, value := range vector {
		projection += int32(splitCoordinate(value)) * weights[dimension]
	}
	return projection
}

func lshCoarseSignature(vector dataset.PackedVector) uint32 {
	var signature uint32
	for dimension := 0; dimension < lshGridDimensions; dimension++ {
		coordinate := splitCoordinate(vector[dimension])
		if coordinate < 0 {
			coordinate = 0
		}
		bin := uint32(coordinate) >> (16 - lshGridBits)
		signature |= bin << (dimension * lshGridBits)
	}
	return signature
}

func lshSignature(vector dataset.PackedVector, planes *[lshBits][model.VectorDimensions]int8) uint64 {
	const center = int64(dataset.PackedScale / 2)
	var signature uint64
	for bit := range planes {
		var sum int64
		for dimension, value := range vector {
			coordinate := splitCoordinate(value)
			sum += int64(planes[bit][dimension]) * (coordinate - center)
		}
		if sum >= 0 {
			signature |= uint64(1) << bit
		}
	}
	return signature
}

func lshProbeMasks(radius int) []uint64 {
	probes := make([]uint64, 0, 1+lshBits+(lshBits*(lshBits-1))/2)
	probes = append(probes, 0)
	if radius >= 1 {
		for bit := 0; bit < lshBits; bit++ {
			probes = append(probes, uint64(1)<<bit)
		}
	}
	if radius >= 2 {
		for left := 0; left < lshBits; left++ {
			for right := left + 1; right < lshBits; right++ {
				probes = append(probes, (uint64(1)<<left)|(uint64(1)<<right))
			}
		}
	}
	if radius >= 3 {
		for first := 0; first < lshBits; first++ {
			for second := first + 1; second < lshBits; second++ {
				for third := second + 1; third < lshBits; third++ {
					probes = append(probes, (uint64(1)<<first)|(uint64(1)<<second)|(uint64(1)<<third))
				}
			}
		}
	}
	return probes
}
