// Package httpapi contém somente a borda HTTP da aplicação.
package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"rinha-backend-2026/internal/model"
)

const maxRequestBody = 1 << 20

// Scorer é a interface mínima que o handler precisa. O handler não conhece
// vectorização, busca ou regras; isso facilita trocar o motor em testes.
type Scorer interface {
	Score(ctx context.Context, request model.FraudRequest) (model.Decision, error)
}

// Handler traduz JSON HTTP para o caso de uso e a decisão de volta para JSON.
type Handler struct {
	scorer   Scorer
	instance string
}

// NewHandler cria um handler. instance mantém o header de diagnóstico usado no
// setup local para observar o round-robin do Nginx; ele não faz parte do corpo
// do contrato da Rinha.
func NewHandler(scorer Scorer, instance string) *Handler {
	return &Handler{scorer: scorer, instance: instance}
}

// ServeHTTP implementa http.Handler e processa uma única requisição válida.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.scorer == nil {
		http.Error(w, "fraud scorer unavailable", http.StatusInternalServerError)
		return
	}

	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)

	var request model.FraudRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		writeClientError(w, fmt.Errorf("decode request: %w", err))
		return
	}
	if err := rejectTrailingJSON(decoder); err != nil {
		writeClientError(w, err)
		return
	}

	decision, err := h.scorer.Score(r.Context(), request)
	if err != nil {
		http.Error(w, "fraud scorer failed", http.StatusInternalServerError)
		return
	}

	if h.instance != "" {
		w.Header().Set("X-API-Instance", h.instance)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	// FraudResponse mantém o contrato público separado do tipo interno Decision.
	_ = json.NewEncoder(w).Encode(model.FraudResponse{
		Approved:   decision.Approved,
		FraudScore: decision.FraudScore,
	})
}

func writeClientError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusBadRequest)
}

func rejectTrailingJSON(decoder *json.Decoder) error {
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("invalid trailing JSON: %w", err)
	}
	return fmt.Errorf("request must contain a single JSON value")
}
