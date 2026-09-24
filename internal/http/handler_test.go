package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rinha-backend-2026/internal/model"
)

func TestRouterReadyState(t *testing.T) {
	readiness := NewReadiness(false)
	router := NewRouter(&staticScorer{}, readiness, "api-test")

	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("not-ready status = %d, want 503", response.Code)
	}

	readiness.SetReady(true)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("ready status = %d, want 200", response.Code)
	}
}

func TestHandlerScoresValidPayload(t *testing.T) {
	scorer := &staticScorer{decision: model.Decision{Approved: false, FraudScore: 0.6}}
	handler := NewHandler(scorer, "api-test")
	request := httptest.NewRequest(http.MethodPost, "/fraud-score", strings.NewReader(`{"id":"tx-1"}`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("X-API-Instance") != "api-test" {
		t.Errorf("instance header = %q, want api-test", response.Header().Get("X-API-Instance"))
	}

	var got model.Decision
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got != scorer.decision {
		t.Errorf("decision = %#v, want %#v", got, scorer.decision)
	}
	if scorer.calls != 1 {
		t.Errorf("scorer calls = %d, want 1", scorer.calls)
	}
}

func TestHandlerRejectsInvalidJSON(t *testing.T) {
	scorer := &staticScorer{}
	handler := NewHandler(scorer, "")
	request := httptest.NewRequest(http.MethodPost, "/fraud-score", strings.NewReader("{"))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
	if scorer.calls != 0 {
		t.Errorf("scorer calls = %d, want 0", scorer.calls)
	}
}

func TestHandlerRejectsTrailingJSON(t *testing.T) {
	handler := NewHandler(&staticScorer{}, "")
	request := httptest.NewRequest(http.MethodPost, "/fraud-score", strings.NewReader(`{} {}`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}

type staticScorer struct {
	decision model.Decision
	err      error
	calls    int
}

func (s *staticScorer) Score(context.Context, model.FraudRequest) (model.Decision, error) {
	s.calls++
	return s.decision, s.err
}
