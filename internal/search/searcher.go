// Package search encontra referências próximas sem tomar decisões de negócio.
package search

import (
	"context"

	"rinha-backend-2026/internal/model"
)

// Searcher é o contrato consumido pelo motor de regras. O algoritmo pode ser
// trocado depois sem fazer o handler conhecer a representação do dataset.
type Searcher interface {
	TopK(ctx context.Context, query model.Vector, k int) ([]model.Neighbor, error)
}
