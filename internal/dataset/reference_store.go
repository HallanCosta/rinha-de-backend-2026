package dataset

import (
	"fmt"
	"math"

	"rinha-backend-2026/internal/model"
)

const (
	// Um bloco evita a realocação em dobro de um slice que pode ter milhões de
	// elementos. Cada referência ocupa 14 uint16 e um byte de label.
	referenceChunkSize = 1 << 16

	// O valor máximo fica reservado para o sentinela -1. Os valores normalizados
	// usam o intervalo [0, 65534], preservando muito mais precisão que float32
	// sem armazenar uma alocação Go por referência.
	packedScale    = PackedScale
	packedSentinel = uint16(65535)

	packedLabelLegit = uint8(0)
	packedLabelFraud = uint8(1)
)

// referenceChunk é a representação compacta de um bloco de referências.
// vectors é um array achatado: [vetor0_dim0, vetor0_dim1, ..., vetor1_dim0].
type referenceChunk struct {
	vectors []uint16
	labels  []uint8
	length  int
}

// NewMemoryReferenceStore cria um store compacto a partir de referências já
// tipadas. A cópia permite que o chamador descarte a slice original sem
// alterar o store.
func NewMemoryReferenceStore(references []model.Reference) (*MemoryReferenceStore, error) {
	store := &MemoryReferenceStore{}
	for index, reference := range references {
		if err := store.append(reference); err != nil {
			return nil, fmt.Errorf("reference %d: %w", index, err)
		}
	}
	return store, nil
}

// append valida e codifica uma referência sem manter o array float64 original.
func (s *MemoryReferenceStore) append(reference model.Reference) error {
	if !reference.Label.Valid() {
		return fmt.Errorf("unsupported label %q", reference.Label)
	}
	if err := validateVector(reference.Vector); err != nil {
		return err
	}

	if len(s.chunks) == 0 || s.chunks[len(s.chunks)-1].length == referenceChunkSize {
		s.chunks = append(s.chunks, referenceChunk{
			vectors: make([]uint16, referenceChunkSize*model.VectorDimensions),
			labels:  make([]uint8, referenceChunkSize),
		})
	}

	chunk := &s.chunks[len(s.chunks)-1]
	position := chunk.length
	base := position * model.VectorDimensions
	for dimension, value := range reference.Vector {
		chunk.vectors[base+dimension] = encodeValue(value)
	}
	chunk.labels[position] = encodeLabel(reference.Label)
	chunk.length++
	s.length++
	return nil
}

// Len retorna a quantidade de referências carregadas no store.
func (s *MemoryReferenceStore) Len() int {
	// Métodos com receiver ponteiro podem tratar nil explicitamente, evitando
	// panic quando uma dependência opcional ainda não foi inicializada.
	if s == nil {
		return 0
	}
	return s.length
}

// At copia um vetor de referência para dst e retorna seu label.
//
// dst também é ponteiro para funcionar como um parâmetro de saída opcional:
// quem só precisa do label pode passar nil. O vetor é decodificado sob demanda,
// mantendo o dataset compacto enquanto a busca percorre seus registros.
func (s *MemoryReferenceStore) At(index int, dst *model.Vector) (model.Label, bool) {
	vector, label, ok := s.AtValue(index)
	if !ok {
		return "", false
	}
	if dst != nil {
		*dst = vector
	}
	return label, true
}

// PackedAt devolve a representação quantizada de uma referência sem
// descompactar seus 14 valores para float64. A KD-tree usa esse caminho para
// comparar o eixo de corte durante a busca sem criar uma cópia grande do
// dataset no índice.
func (s *MemoryReferenceStore) PackedAt(index int) (PackedVector, model.Label, bool) {
	if s == nil || index < 0 || index >= s.length {
		return PackedVector{}, "", false
	}

	chunk := &s.chunks[index/referenceChunkSize]
	position := index % referenceChunkSize
	base := position * model.VectorDimensions
	var vector PackedVector
	copy(vector[:], chunk.vectors[base:base+model.VectorDimensions])
	return vector, decodeLabel(chunk.labels[position]), true
}

// AtValue é a variante sem parâmetro de saída usada pelo caminho otimizado da
// busca. O array fixo é devolvido por valor e não cria uma alocação por item.
func (s *MemoryReferenceStore) AtValue(index int) (model.Vector, model.Label, bool) {
	packed, label, ok := s.PackedAt(index)
	if !ok {
		return model.Vector{}, "", false
	}

	var vector model.Vector
	for dimension, value := range packed {
		vector[dimension] = decodeValue(value)
	}
	return vector, label, true
}

// DistanceKeyAt calcula a distância quantizada diretamente no bloco, sem
// copiar ou decodificar o vetor de referência.
func (s *MemoryReferenceStore) DistanceKeyAt(index int, query PackedVector) (uint64, model.Label, bool) {
	if s == nil || index < 0 || index >= s.length {
		return 0, "", false
	}

	chunk := &s.chunks[index/referenceChunkSize]
	position := index % referenceChunkSize
	base := position * model.VectorDimensions
	var sum uint64
	for dimension, value := range query {
		difference := packedDifferenceKey(value, chunk.vectors[base+dimension])
		sum += uint64(difference * difference)
	}
	return sum, decodeLabel(chunk.labels[position]), true
}

// EncodeVector aplica a mesma codificação do store ao vetor de consulta.
func EncodeVector(vector model.Vector) PackedVector {
	var encoded PackedVector
	for dimension, value := range vector {
		encoded[dimension] = encodeValue(value)
	}
	return encoded
}

// PackedDistanceSquared calcula a distância euclidiana ao quadrado no espaço
// quantizado. O sentinela -1 precisa de tratamento próprio: numericamente ele
// fica fora do intervalo dos códigos normalizados 0..1.
func PackedDistanceSquared(left, right PackedVector) float64 {
	return float64(PackedDistanceKey(left, right)) / (packedScale * packedScale)
}

// PackedDistanceKey retorna a distância ao quadrado sem divisões por dimensão.
// Como todos os candidatos usam a mesma escala, comparar essa chave preserva
// a ordenação e deixa a conversão para float64 somente nos cinco resultados.
func PackedDistanceKey(left, right PackedVector) uint64 {
	return packedDistanceKey(left, right)
}

func packedDifferenceKey(left, right uint16) int64 {
	if left == packedSentinel {
		if right == packedSentinel {
			return 0
		}
		return -int64(packedScale) - int64(right)
	}
	if right == packedSentinel {
		return int64(packedScale) + int64(left)
	}
	return int64(left) - int64(right)
}

// References retorna uma cópia descompactada das referências. Serve para
// testes e inspeção; o código de busca deve depender de ReferenceStore.
func (s *MemoryReferenceStore) References() []model.Reference {
	if s == nil {
		return nil
	}

	result := make([]model.Reference, 0, s.length)
	for index := 0; index < s.length; index++ {
		var vector model.Vector
		label, _ := s.At(index, &vector)
		result = append(result, model.Reference{Vector: vector, Label: label})
	}
	return result
}

func validateVector(vector model.Vector) error {
	for dimension, value := range vector {
		if value == -1 {
			if dimension != 5 && dimension != 6 {
				return fmt.Errorf("dimension %d: sentinel -1 is only valid at dimensions 5 and 6", dimension)
			}
			continue
		}
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
			return fmt.Errorf("dimension %d: value %v outside [0, 1]", dimension, value)
		}
	}
	return nil
}

func encodeValue(value float64) uint16 {
	if value == -1 {
		return packedSentinel
	}
	return uint16(math.Round(value * packedScale))
}

func decodeValue(value uint16) float64 {
	if value == packedSentinel {
		return -1
	}
	return float64(value) / packedScale
}

func encodeLabel(label model.Label) uint8 {
	if label == model.LabelFraud {
		return packedLabelFraud
	}
	return packedLabelLegit
}

func decodeLabel(label uint8) model.Label {
	if label == packedLabelFraud {
		return model.LabelFraud
	}
	return model.LabelLegit
}
