// Package dataset carrega os dados estáticos de referência usados pelo futuro
// motor de fraude. Ele não faz vetorização nem busca de vizinhos.
package dataset

import "rinha-backend-2026/internal/model"

// Paths identifica os três arquivos de referência descritos pelo desafio.
//
// São caminhos de configuração, e não dados do domínio. Por isso esta struct
// não precisa de tags JSON: ela é usada internamente para montar o loader.
type Paths struct {
	ReferencesPath    string
	MCCRiskPath       string
	NormalizationPath string
}

// Normalization contém as constantes usadas nas 14 dimensões documentadas.
// Os valores ficam em uma struct tipada para evitar mapas com chaves digitadas
// manualmente durante a futura vetorização.
type Normalization struct {
	MaxAmount            float64
	MaxInstallments      float64
	AmountVsAvgRatio     float64
	MaxMinutes           float64
	MaxKM                float64
	MaxTxCount24h        float64
	MaxMerchantAvgAmount float64
}

// Metadata contém os pequenos metadados usados na futura vetorização.
// Um map[string]float64 é semelhante a Record<string, number> em TypeScript.
type Metadata struct {
	MCCRisk       map[string]float64
	Normalization Normalization
}

const defaultMCCRisk = 0.5

// RiskFor retorna o fallback documentado quando um MCC não está presente.
func (m Metadata) RiskFor(mcc string) float64 {
	// O segundo valor do retorno do map (ok) distingue "chave ausente" de um
	// risco armazenado como zero; essa convenção é chamada de comma-ok em Go.
	if risk, ok := m.MCCRisk[mcc]; ok {
		return risk
	}
	return defaultMCCRisk
}

// ReferenceStore é a fronteira somente-leitura que a futura busca usará.
//
// Interfaces em Go descrevem o comportamento necessário, e não uma hierarquia
// de classes. Assim, a busca poderá receber tanto este store em memória quanto
// outra implementação compacta/indexada sem mudar seu contrato.
type ReferenceStore interface {
	Len() int
	At(index int, dst *model.Vector) (label model.Label, ok bool)
}

// ValueReferenceStore é uma extensão opcional para stores que conseguem
// devolver o vetor por valor. O searcher usa essa forma no caminho quente para
// evitar que um ponteiro temporário escape para o heap a cada referência.
type ValueReferenceStore interface {
	ReferenceStore
	AtValue(index int) (vector model.Vector, label model.Label, ok bool)
}

// PackedVector é o formato interno compactado usado pelo dataset. A busca
// pode consumir esse formato diretamente para evitar decodificar float64 em
// cada uma das milhões de referências.
type PackedVector [model.VectorDimensions]uint16

// PackedScale é a escala dos valores normalizados dentro de PackedVector.
const PackedScale = 65534.0

// PackedReferenceStore é a extensão usada pelo caminho otimizado do brute
// force. Implementações que não usam quantização continuam podendo atender
// somente ReferenceStore ou ValueReferenceStore.
type PackedReferenceStore interface {
	ReferenceStore
	DistanceKeyAt(index int, query PackedVector) (distanceKey uint64, label model.Label, ok bool)
}

// Dataset agrupa referências e metadados carregados para futura injeção de
// dependência no bootstrap da aplicação.
type Dataset struct {
	References ReferenceStore
	Metadata   Metadata
}

// MemoryReferenceStore é um store somente-leitura em memória, adequado para
// fixtures e para o dataset de produção. Os vetores são armazenados em blocos
// compactos; o formato público continua sendo o contrato ReferenceStore.
type MemoryReferenceStore struct {
	// Os campos são privados para impedir que consumidores alterem a
	// representação diretamente; eles dependem da interface ReferenceStore.
	chunks []referenceChunk
	length int
}
