package model

// Decision é o resultado do motor de fraude que atravessa as camadas internas
// até o handler HTTP. As tags mantêm exatamente o contrato da resposta pública.
type Decision struct {
	Approved   bool    `json:"approved"`
	FraudScore float64 `json:"fraud_score"`
}
