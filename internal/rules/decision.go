package rules

import (
	"fmt"

	"rinha-backend-2026/internal/model"
)

const fraudThreshold = 0.6

// Decide calcula somente a regra oficial: fraudes/5 e aprovação abaixo de
// 0.6. Exigir exatamente cinco vizinhos evita produzir score com outro
// denominador quando o dataset estiver incompleto.
func Decide(neighbors []model.Neighbor) (model.Decision, error) {
	if len(neighbors) != neighborsCount {
		return model.Decision{}, fmt.Errorf("expected %d neighbors, got %d", neighborsCount, len(neighbors))
	}

	fraudCount := 0
	for _, neighbor := range neighbors {
		if neighbor.IsFraud() {
			fraudCount++
		}
	}

	score := float64(fraudCount) / float64(neighborsCount)
	return model.Decision{
		Approved:   score < fraudThreshold,
		FraudScore: score,
	}, nil
}
