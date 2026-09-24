// Package model reúne os tipos de domínio compartilhados pelo futuro motor de fraude.
package model

import "time"

// FraudRequest é o payload aceito pelo POST /fraud-score.
//
// Em Go, uma struct descreve um objeto com campos tipados, de forma parecida
// com um type/interface em TypeScript. As tags json dizem ao pacote encoding/json
// qual nome usar na entrada e na saída, independentemente do nome do campo Go.
type FraudRequest struct {
	ID          string      `json:"id"`
	Transaction Transaction `json:"transaction"`
	Customer    Customer    `json:"customer"`
	Merchant    Merchant    `json:"merchant"`
	Terminal    Terminal    `json:"terminal"`
	// O ponteiro permite distinguir "objeto ausente/nulo" de um objeto vazio.
	// É o equivalente prático de uma propriedade opcional em TypeScript.
	LastTransaction *LastTransaction `json:"last_transaction"`
}

// Transaction contém as informações do pagamento atual.
type Transaction struct {
	Amount       float64 `json:"amount"`
	Installments int     `json:"installments"`
	// time.Time representa um instante; o decoder JSON espera o formato de
	// data/hora documentado, em vez de deixar a data como uma string solta.
	RequestedAt time.Time `json:"requested_at"`
}

// Customer contém o histórico relevante do portador do cartão.
// []string é uma slice: uma lista cujo tamanho pode variar, como string[] em
// TypeScript. A ordem recebida é preservada.
type Customer struct {
	AvgAmount      float64  `json:"avg_amount"`
	TxCount24h     int      `json:"tx_count_24h"`
	KnownMerchants []string `json:"known_merchants"`
}

// Merchant contém os dados do estabelecimento do pagamento.
type Merchant struct {
	ID        string  `json:"id"`
	MCC       string  `json:"mcc"`
	AvgAmount float64 `json:"avg_amount"`
}

// Terminal contém as propriedades do terminal usado no pagamento.
type Terminal struct {
	IsOnline    bool    `json:"is_online"`
	CardPresent bool    `json:"card_present"`
	KmFromHome  float64 `json:"km_from_home"`
}

// LastTransaction contém os dados opcionais do pagamento anterior.
type LastTransaction struct {
	Timestamp     time.Time `json:"timestamp"`
	KmFromCurrent float64   `json:"km_from_current"`
}

// FraudResponse é a resposta de sucesso do POST /fraud-score.
//
// O tipo mantém o contrato HTTP separado da implementação do motor: no futuro
// a regra poderá mudar sem fazer o handler depender dos detalhes do cálculo.
type FraudResponse struct {
	Approved   bool    `json:"approved"`
	FraudScore float64 `json:"fraud_score"`
}
