// Package vectorizer transforma o payload HTTP em um vetor de domínio.
package vectorizer

import (
	"fmt"
	"time"

	"rinha-backend-2026/internal/dataset"
	"rinha-backend-2026/internal/model"
)

// Vectorizer é a fronteira usada pelo motor de regras. A interface permite
// testar o engine com um vetor mockado sem depender de datas ou metadados reais.
type Vectorizer interface {
	Vectorize(request model.FraudRequest) (model.Vector, error)
}

// Engine é a implementação determinística das 14 dimensões documentadas.
// Depois de criado, seus metadados são somente leitura e podem ser
// compartilhados por todas as requisições da instância.
type Engine struct {
	metadata        dataset.Metadata
	validationError error
}

// New cria um vectorizer com os metadados carregados no startup.
func New(metadata dataset.Metadata) *Engine {
	// A validação acontece uma vez no bootstrap. Repeti-la para cada request
	// desperdiçaria trabalho em um caminho que só lê metadados imutáveis.
	return &Engine{
		metadata:        metadata,
		validationError: validateNormalization(metadata.Normalization),
	}
}

// Vectorize aplica as fórmulas de regra de detecção na ordem exata do
// contrato. Não acessa HTTP, arquivos ou o dataset de referências.
func (e *Engine) Vectorize(request model.FraudRequest) (model.Vector, error) {
	if e == nil {
		return model.Vector{}, fmt.Errorf("vectorizer is nil")
	}
	if e.validationError != nil {
		return model.Vector{}, e.validationError
	}

	transaction := request.Transaction
	normalization := e.metadata.Normalization
	vector := model.Vector{}

	vector[0] = clamp(transaction.Amount / normalization.MaxAmount)
	vector[1] = clamp(float64(transaction.Installments) / normalization.MaxInstallments)

	amountVsAverage, err := amountVsAverage(transaction.Amount, request.Customer.AvgAmount, normalization.AmountVsAvgRatio)
	if err != nil {
		return model.Vector{}, err
	}
	vector[2] = amountVsAverage

	requestedAtUTC := transaction.RequestedAt.UTC()
	vector[3] = float64(requestedAtUTC.Hour()) / 23
	vector[4] = dayOfWeek(requestedAtUTC) / 6

	// -1 é um sentinela documentado, não um valor que deve passar pelo clamp.
	vector[5] = -1
	vector[6] = -1
	if last := request.LastTransaction; last != nil {
		minutes := requestedAtUTC.Sub(last.Timestamp.UTC()).Minutes()
		vector[5] = clamp(minutes / normalization.MaxMinutes)
		vector[6] = clamp(last.KmFromCurrent / normalization.MaxKM)
	}

	vector[7] = clamp(request.Terminal.KmFromHome / normalization.MaxKM)
	vector[8] = clamp(float64(request.Customer.TxCount24h) / normalization.MaxTxCount24h)
	vector[9] = boolValue(request.Terminal.IsOnline)
	vector[10] = boolValue(request.Terminal.CardPresent)
	if !knownMerchant(request.Customer.KnownMerchants, request.Merchant.ID) {
		vector[11] = 1
	}
	vector[12] = e.metadata.RiskFor(request.Merchant.MCC)
	vector[13] = clamp(request.Merchant.AvgAmount / normalization.MaxMerchantAvgAmount)

	return vector, nil
}

func amountVsAverage(amount, average, ratio float64) (float64, error) {
	if average == 0 {
		return 0, fmt.Errorf("customer.avg_amount must not be zero")
	}
	return clamp((amount / average) / ratio), nil
}

func validateNormalization(normalization dataset.Normalization) error {
	values := []struct {
		name  string
		value float64
	}{
		{"max_amount", normalization.MaxAmount},
		{"max_installments", normalization.MaxInstallments},
		{"amount_vs_avg_ratio", normalization.AmountVsAvgRatio},
		{"max_minutes", normalization.MaxMinutes},
		{"max_km", normalization.MaxKM},
		{"max_tx_count_24h", normalization.MaxTxCount24h},
		{"max_merchant_avg_amount", normalization.MaxMerchantAvgAmount},
	}
	for _, item := range values {
		if item.value <= 0 {
			return fmt.Errorf("normalization %q must be positive", item.name)
		}
	}
	return nil
}

// Mantém os valores normalizados no intervalo exigido pela especificação.
func clamp(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

// dayOfWeek converte o Weekday de Go (domingo=0) para a convenção do desafio
// (segunda=0, ..., domingo=6), sempre usando o instante em UTC.
func dayOfWeek(value time.Time) float64 {
	mondayFirst := (int(value.Weekday()) + 6) % 7
	return float64(mondayFirst)
}

func boolValue(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func knownMerchant(known []string, merchantID string) bool {
	for _, knownID := range known {
		if knownID == merchantID {
			return true
		}
	}
	return false
}
