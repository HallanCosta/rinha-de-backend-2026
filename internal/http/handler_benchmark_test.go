package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"rinha-backend-2026/internal/model"
)

var benchmarkFraudPayload = []byte(`{
  "id": "tx-benchmark",
  "transaction": {"amount": 41.12, "installments": 2, "requested_at": "2026-03-11T18:45:53Z"},
  "customer": {"avg_amount": 82.24, "tx_count_24h": 3, "known_merchants": ["MERC-003", "MERC-016"]},
  "merchant": {"id": "MERC-016", "mcc": "5411", "avg_amount": 60.25},
  "terminal": {"is_online": false, "card_present": true, "km_from_home": 29.23},
  "last_transaction": null
}`)

func BenchmarkHandlerServeHTTP(b *testing.B) {
	handler := NewHandler(&staticScorer{
		decision: model.Decision{Approved: true, FraudScore: 0},
	}, "")
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		request := httptest.NewRequest(http.MethodPost, "/fraud-score", bytes.NewReader(benchmarkFraudPayload))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			b.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
		}
	}
}
