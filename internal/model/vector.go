package model

import (
	"encoding/json"
	"fmt"
)

// VectorDimensions é a quantidade de dimensões especificada pelo desafio.
const VectorDimensions = 14

// Vector é a representação normalizada de uma transação.
//
// Os colchetes indicam um array de tamanho fixo, diferente de []float64:
// qualquer Vector sempre tem exatamente 14 posições. As dimensões 5 e 6 podem
// conter -1 quando não existe uma última transação.
type Vector [VectorDimensions]float64

// UnmarshalJSON rejeita vetores de referência com quantidade de dimensões
// diferente do contrato, em vez de truncar ou preencher o array silenciosamente.
//
// O receiver é um ponteiro (*Vector) porque o decoder precisa alterar o valor
// recebido. Isso se parece com passar um objeto mutável por referência, mas é
// uma escolha explícita da assinatura do método em Go.
func (v *Vector) UnmarshalJSON(data []byte) error {
	// A slice temporária permite validar len antes de copiar para o array fixo.
	var values []float64
	if err := json.Unmarshal(data, &values); err != nil {
		return fmt.Errorf("decode vector: %w", err)
	}
	if len(values) != VectorDimensions {
		return fmt.Errorf("decode vector: expected %d dimensions, got %d", VectorDimensions, len(values))
	}

	copy(v[:], values)
	return nil
}

// Label identifica a classe atribuída a uma transação de referência.
type Label string

const (
	// LabelFraud é a classe de fraude de references.json.gz.
	LabelFraud Label = "fraud"
	// LabelLegit é a classe legítima de references.json.gz.
	LabelLegit Label = "legit"
)

// Valid informa se o label é uma das duas classes documentadas.
func (l Label) Valid() bool {
	return l == LabelFraud || l == LabelLegit
}

// UnmarshalJSON rejeita labels fora das classes fraud/legit documentadas.
func (l *Label) UnmarshalJSON(data []byte) error {
	// O tipo string é decodificado primeiro; depois fazemos a conversão explícita
	// para Label e validamos o domínio permitido.
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("decode reference label: %w", err)
	}

	decoded := Label(value)
	if !decoded.Valid() {
		return fmt.Errorf("decode reference label: unsupported value %q", value)
	}

	*l = decoded
	return nil
}

// Reference é um vetor rotulado do dataset de referência.
type Reference struct {
	Vector Vector `json:"vector"`
	Label  Label  `json:"label"`
}

// Neighbor é o resultado mínimo que a futura busca top-k e as regras precisarão.
type Neighbor struct {
	Distance float64
	Label    Label
}

// IsFraud informa se o vizinho recebeu o label de fraude.
func (n Neighbor) IsFraud() bool {
	return n.Label == LabelFraud
}
