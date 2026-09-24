// Package rules orquestra a classificação, sem conhecer HTTP ou arquivos.
package rules

import (
	"context"
	"fmt"

	"rinha-backend-2026/internal/model"
	"rinha-backend-2026/internal/search"
	"rinha-backend-2026/internal/vectorizer"
)

const neighborsCount = 5

// Engine é o caso de uso da detecção: vetoriza, busca cinco vizinhos e aplica
// a decisão documentada. Ele não abre o dataset nem serializa JSON.
type Engine struct {
	vectorizer vectorizer.Vectorizer
	searcher   search.Searcher
}

// New conecta as dependências concretas atrás de interfaces pequenas.
func New(vectorizer vectorizer.Vectorizer, searcher search.Searcher) *Engine {
	return &Engine{
		vectorizer: vectorizer,
		searcher:   searcher,
	}
}

// Score executa uma classificação completa para uma requisição.
func (e *Engine) Score(ctx context.Context, request model.FraudRequest) (model.Decision, error) {
	if e == nil || e.vectorizer == nil || e.searcher == nil {
		return model.Decision{}, fmt.Errorf("rules engine dependencies are incomplete")
	}

	query, err := e.vectorizer.Vectorize(request)
	if err != nil {
		return model.Decision{}, fmt.Errorf("vectorize request: %w", err)
	}

	neighbors, err := e.searcher.TopK(ctx, query, neighborsCount)
	if err != nil {
		return model.Decision{}, fmt.Errorf("search nearest neighbors: %w", err)
	}

	decision, err := Decide(neighbors)
	if err != nil {
		return model.Decision{}, err
	}
	return decision, nil
}
