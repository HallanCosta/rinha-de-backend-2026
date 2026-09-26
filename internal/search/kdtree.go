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
	kdTreeNilNode = int32(-1)

	// O índice ocupa 28 bits; os quatro bits superiores guardam a dimensão
	// de corte. O dataset oficial fica muito abaixo de 2^28 referências.
	kdTreeIndexMask = uint32((1 << 28) - 1)
	kdTreeMaxIndex  = int64(kdTreeIndexMask)
)

// KDTree é um índice exato para vizinhos mais próximos em distância
// euclidiana. O dataset é somente leitura depois da construção, então a
// estrutura pode ser compartilhada por todas as goroutines do servidor.
//
// A árvore não duplica os vetores: cada nó guarda apenas o índice da referência
// no MemoryReferenceStore. Os vetores continuam no formato PackedVector
// quantizado e são lidos quando um nó é visitado.
type KDTree struct {
	references    dataset.ReferenceStore
	distanceStore packedDistanceStore
	nodes         []kdNode
	splitValues   []uint16
	root          int32
	maxVisited    int
}

// kdNode representa um ponto e os dois subespaços criados pelo corte.
// referenceAndDimension compacta o índice da referência e a dimensão em um
// uint32 para manter o índice em 12 bytes por nó, em vez de desperdiçar
// alinhamento com um campo separado de dimensão.
type kdNode struct {
	referenceAndDimension uint32
	left                  int32
	right                 int32
}

// kdPoint existe somente durante a construção. Depois que a mediana de cada
// subárvore é escolhida, os vetores temporários são liberados e a árvore
// conserva apenas os índices dos registros.
type kdPoint struct {
	index  int32
	vector dataset.PackedVector
}

type kdBounds struct {
	min [model.VectorDimensions]int64
	max [model.VectorDimensions]int64
}

type kdBranch struct {
	node       int32
	lowerBound uint64
}

type kdBranchHeap []kdBranch

func (h kdBranchHeap) Len() int { return len(h) }

func (h kdBranchHeap) Less(left, right int) bool {
	if h[left].lowerBound != h[right].lowerBound {
		return h[left].lowerBound < h[right].lowerBound
	}
	return h[left].node < h[right].node
}

func (h kdBranchHeap) Swap(left, right int) { h[left], h[right] = h[right], h[left] }

func (h *kdBranchHeap) Push(value any) {
	*h = append(*h, value.(kdBranch))
}

func (h *kdBranchHeap) Pop() any {
	old := *h
	last := len(old) - 1
	value := old[last]
	*h = old[:last]
	return value
}

// packedValueStore é uma extensão opcional do store compacto. O fallback usa
// ReferenceStore.At e recompacta o vetor, mantendo a KD-tree útil para stores
// de teste que não conhecem o formato PackedVector.
type packedValueStore interface {
	PackedAt(index int) (vector dataset.PackedVector, label model.Label, ok bool)
}

// packedDistanceStore evita descompactar o vetor inteiro durante a busca.
// O store oficial consegue calcular a distância diretamente no bloco de
// uint16 e devolver apenas a chave e o label necessários ao TopK.
type packedDistanceStore interface {
	DistanceKeyAt(index int, query dataset.PackedVector) (distance uint64, label model.Label, ok bool)
}

// NewKDTree constrói uma árvore balanceada a partir de um store somente-leitura.
// A construção acontece no startup, fora do caminho de cada requisição.
func NewKDTree(references dataset.ReferenceStore) (*KDTree, error) {
	return newKDTree(references, 0)
}

// NewKDTreeWithVisitLimit cria uma busca aproximada com orçamento fixo de
// nós. Em dimensões altas, uma KD-tree exata pode visitar uma fração grande do
// dataset; limitar o trabalho mantém a latência previsível e segue a margem
// ANN permitida pela documentação da Rinha.
func NewKDTreeWithVisitLimit(references dataset.ReferenceStore, maxVisited int) (*KDTree, error) {
	if maxVisited <= 0 {
		return nil, fmt.Errorf("max visited must be positive")
	}
	return newKDTree(references, maxVisited)
}

func newKDTree(references dataset.ReferenceStore, maxVisited int) (*KDTree, error) {
	if references == nil {
		return nil, fmt.Errorf("reference store is nil")
	}
	if int64(references.Len()) > kdTreeMaxIndex {
		return nil, fmt.Errorf("reference store has too many items: %d", references.Len())
	}

	points := make([]kdPoint, references.Len())
	for index := range points {
		vector, _, ok := packedAt(references, index)
		if !ok {
			return nil, fmt.Errorf("reference store returned no item at index %d", index)
		}
		points[index] = kdPoint{index: int32(index), vector: vector}
	}

	distanceStore, _ := references.(packedDistanceStore)
	tree := &KDTree{
		references:    references,
		distanceStore: distanceStore,
		nodes:         make([]kdNode, 0, len(points)),
		splitValues:   make([]uint16, 0, len(points)),
		root:          kdTreeNilNode,
		maxVisited:    maxVisited,
	}
	tree.root = tree.build(points, 0)
	return tree, nil
}

// TopK retorna até k vizinhos em ordem crescente. A poda usa a distância até
// o hiperplano do nó: se essa distância já for pior que o pior candidato
// atual, nenhum ponto do outro lado pode melhorar o resultado.
func (s *KDTree) TopK(ctx context.Context, query model.Vector, k int) ([]model.Neighbor, error) {
	if s == nil || s.references == nil {
		return nil, fmt.Errorf("kd-tree reference store is nil")
	}
	if k <= 0 {
		return nil, fmt.Errorf("k must be positive")
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if s.root == kdTreeNilNode {
		return []model.Neighbor{}, nil
	}

	queryPacked := dataset.EncodeVector(query)
	candidates := make(candidateHeap, 0, min(k, len(s.nodes)))
	heap.Init(&candidates)
	visited := 0
	var err error
	if s.maxVisited > 0 {
		err = s.visitLimited(ctx, s.root, queryPacked, k, &candidates, &visited)
	} else {
		err = s.visit(ctx, s.root, queryPacked, k, &candidates, &visited)
	}
	if err != nil {
		return nil, err
	}
	return finishCandidates(ctx, candidates)
}

func (s *KDTree) visitLimited(
	ctx context.Context,
	nodeIndex int32,
	query dataset.PackedVector,
	k int,
	candidates *candidateHeap,
	visited *int,
) error {
	if nodeIndex == kdTreeNilNode {
		return nil
	}
	branches := make(kdBranchHeap, 0, 64)
	heap.Push(&branches, kdBranch{node: nodeIndex})
	for branches.Len() > 0 {
		branch := heap.Pop(&branches).(kdBranch)
		if *visited >= s.maxVisited && candidates.Len() >= k {
			return nil
		}
		if candidates.Len() >= k && branch.lowerBound > uint64((*candidates)[0].distanceSquared) {
			return nil
		}
		if *visited&1023 == 0 {
			if err := contextError(ctx); err != nil {
				return err
			}
		}
		*visited++

		node := s.nodes[branch.node]
		referenceIndex := unpackNodeReference(node.referenceAndDimension)
		var distanceKey uint64
		var label model.Label
		var ok bool
		if s.distanceStore != nil {
			distanceKey, label, ok = s.distanceStore.DistanceKeyAt(int(referenceIndex), query)
		} else {
			vector, fallbackLabel, fallbackOK := packedAt(s.references, int(referenceIndex))
			distanceKey = dataset.PackedDistanceKey(query, vector)
			label = fallbackLabel
			ok = fallbackOK
		}
		if !ok {
			return fmt.Errorf("reference store returned no item at index %d", referenceIndex)
		}
		pushCandidate(candidates, candidate{
			distanceSquared: float64(distanceKey),
			packed:          true,
			label:           label,
			index:           int(referenceIndex),
		}, k)

		dimension := unpackNodeDimension(node.referenceAndDimension)
		queryValue := splitCoordinate(query[dimension])
		nodeValue := splitCoordinate(s.splitValues[branch.node])
		near, far := node.left, node.right
		if queryValue > nodeValue {
			near, far = node.right, node.left
		}
		if near != kdTreeNilNode {
			heap.Push(&branches, kdBranch{node: near, lowerBound: branch.lowerBound})
		}
		if far != kdTreeNilNode {
			difference := queryValue - nodeValue
			if difference < 0 {
				difference = -difference
			}
			lowerBound := branch.lowerBound
			planeBound := uint64(difference * difference)
			if planeBound > lowerBound {
				lowerBound = planeBound
			}
			heap.Push(&branches, kdBranch{node: far, lowerBound: lowerBound})
		}
	}
	return nil
}

// build escolhe a mediana do eixo atual e recursa nos dois intervalos. A
// ordenação determinística por índice mantém o desempate compatível com o
// brute force quando dois vetores têm exatamente a mesma distância.
func (s *KDTree) build(points []kdPoint, depth int) int32 {
	if len(points) == 0 {
		return kdTreeNilNode
	}

	dimension := uint8(depth % model.VectorDimensions)
	sort.Slice(points, func(left, right int) bool {
		leftValue := splitCoordinate(points[left].vector[dimension])
		rightValue := splitCoordinate(points[right].vector[dimension])
		if leftValue != rightValue {
			return leftValue < rightValue
		}
		return points[left].index < points[right].index
	})

	median := len(points) / 2
	nodeIndex := int32(len(s.nodes))
	s.nodes = append(s.nodes, kdNode{
		referenceAndDimension: packNodeReference(points[median].index, dimension),
		left:                  kdTreeNilNode,
		right:                 kdTreeNilNode,
	})
	s.splitValues = append(s.splitValues, points[median].vector[dimension])

	left := s.build(points[:median], depth+1)
	right := s.build(points[median+1:], depth+1)
	s.nodes[nodeIndex].left = left
	s.nodes[nodeIndex].right = right
	return nodeIndex
}

func (s *KDTree) visit(
	ctx context.Context,
	nodeIndex int32,
	query dataset.PackedVector,
	k int,
	candidates *candidateHeap,
	visited *int,
) error {
	bounds := newKDBounds()
	return s.visitRegion(ctx, nodeIndex, query, k, candidates, visited, &bounds)
}

func (s *KDTree) visitRegion(
	ctx context.Context,
	nodeIndex int32,
	query dataset.PackedVector,
	k int,
	candidates *candidateHeap,
	visited *int,
	bounds *kdBounds,
) error {
	if nodeIndex == kdTreeNilNode {
		return nil
	}
	if s.maxVisited > 0 && *visited >= s.maxVisited && candidates.Len() >= k {
		return nil
	}
	if *visited&1023 == 0 {
		if err := contextError(ctx); err != nil {
			return err
		}
	}
	*visited++

	node := s.nodes[nodeIndex]
	referenceIndex := unpackNodeReference(node.referenceAndDimension)
	var distanceKey uint64
	var label model.Label
	var ok bool
	if s.distanceStore != nil {
		distanceKey, label, ok = s.distanceStore.DistanceKeyAt(int(referenceIndex), query)
	} else {
		vector, fallbackLabel, fallbackOK := packedAt(s.references, int(referenceIndex))
		distanceKey = dataset.PackedDistanceKey(query, vector)
		label = fallbackLabel
		ok = fallbackOK
	}
	if !ok {
		return fmt.Errorf("reference store returned no item at index %d", referenceIndex)
	}

	pushCandidate(candidates, candidate{
		distanceSquared: float64(distanceKey),
		packed:          true,
		label:           label,
		index:           int(referenceIndex),
	}, k)

	dimension := unpackNodeDimension(node.referenceAndDimension)
	queryValue := splitCoordinate(query[dimension])
	nodeValue := splitCoordinate(s.splitValues[nodeIndex])
	near, far := node.left, node.right
	if queryValue > nodeValue {
		near, far = node.right, node.left
	}

	if queryValue <= nodeValue {
		oldMax := bounds.max[dimension]
		bounds.max[dimension] = nodeValue
		if err := s.visitRegion(ctx, near, query, k, candidates, visited, bounds); err != nil {
			return err
		}
		bounds.max[dimension] = oldMax

		oldMin := bounds.min[dimension]
		bounds.min[dimension] = nodeValue
		if candidates.Len() < k || lowerBoundKey(query, bounds) <= uint64((*candidates)[0].distanceSquared) {
			if err := s.visitRegion(ctx, far, query, k, candidates, visited, bounds); err != nil {
				return err
			}
		}
		bounds.min[dimension] = oldMin
	} else {
		oldMin := bounds.min[dimension]
		bounds.min[dimension] = nodeValue
		if err := s.visitRegion(ctx, near, query, k, candidates, visited, bounds); err != nil {
			return err
		}
		bounds.min[dimension] = oldMin

		oldMax := bounds.max[dimension]
		bounds.max[dimension] = nodeValue
		if candidates.Len() < k || lowerBoundKey(query, bounds) <= uint64((*candidates)[0].distanceSquared) {
			if err := s.visitRegion(ctx, far, query, k, candidates, visited, bounds); err != nil {
				return err
			}
		}
		bounds.max[dimension] = oldMax
	}
	return nil
}

func newKDBounds() kdBounds {
	bounds := kdBounds{}
	for dimension := range bounds.min {
		bounds.min[dimension] = 0
		bounds.max[dimension] = int64(dataset.PackedScale)
		if dimension == 5 || dimension == 6 {
			bounds.min[dimension] = -int64(dataset.PackedScale)
		}
	}
	return bounds
}

func lowerBoundKey(query dataset.PackedVector, bounds *kdBounds) uint64 {
	var sum uint64
	for dimension, value := range query {
		queryValue := splitCoordinate(value)
		var difference int64
		if queryValue < bounds.min[dimension] {
			difference = bounds.min[dimension] - queryValue
		} else if queryValue > bounds.max[dimension] {
			difference = queryValue - bounds.max[dimension]
		}
		sum += uint64(difference * difference)
	}
	return sum
}

// packedAt lê a referência no mesmo espaço quantizado usado pelo brute force.
// Para o store oficial isso é uma cópia de 14 uint16; para fixtures genéricas
// há um fallback que converte o Vector público para o formato compacto.
func packedAt(references dataset.ReferenceStore, index int) (dataset.PackedVector, model.Label, bool) {
	if packed, ok := references.(packedValueStore); ok {
		return packed.PackedAt(index)
	}

	var vector model.Vector
	label, ok := references.At(index, &vector)
	if !ok {
		return dataset.PackedVector{}, "", false
	}
	return dataset.EncodeVector(vector), label, true
}

// splitCoordinate traduz o sentinela -1 para o mesmo eixo numérico usado pela
// distância quantizada. O código 65535 representa -1, que fica antes de todos
// os valores normalizados de 0 a 1.
func splitCoordinate(value uint16) int64 {
	if value == ^uint16(0) {
		return -int64(dataset.PackedScale)
	}
	return int64(value)
}

func packNodeReference(index int32, dimension uint8) uint32 {
	return uint32(index) | uint32(dimension)<<28
}

func unpackNodeReference(value uint32) int32 {
	return int32(value & kdTreeIndexMask)
}

func unpackNodeDimension(value uint32) uint8 {
	return uint8(value >> 28)
}
