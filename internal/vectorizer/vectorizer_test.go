package vectorizer

import (
	"math"
	"testing"
	"time"

	"rinha-backend-2026/internal/dataset"
	"rinha-backend-2026/internal/model"
)

func TestVectorizeDocumentedLegitimateTransaction(t *testing.T) {
	requestedAt := time.Date(2026, time.March, 11, 18, 45, 53, 0, time.UTC)
	request := model.FraudRequest{
		Transaction: model.Transaction{
			Amount:       41.12,
			Installments: 2,
			RequestedAt:  requestedAt,
		},
		Customer: model.Customer{
			AvgAmount:      82.24,
			TxCount24h:     3,
			KnownMerchants: []string{"MERC-003", "MERC-016"},
		},
		Merchant: model.Merchant{
			ID:        "MERC-016",
			MCC:       "5411",
			AvgAmount: 60.25,
		},
		Terminal: model.Terminal{
			IsOnline:    false,
			CardPresent: true,
			KmFromHome:  29.23,
		},
	}

	got, err := New(testMetadata()).Vectorize(request)
	if err != nil {
		t.Fatalf("vectorize request: %v", err)
	}

	want := model.Vector{0.004112, 0.1666666667, 0.05, 18.0 / 23.0, 2.0 / 6.0, -1, -1, 0.02923, 0.15, 0, 1, 0, 0.15, 0.006025}
	assertVectorClose(t, got, want)
}

func TestVectorizeDocumentedFraudTransaction(t *testing.T) {
	request := model.FraudRequest{
		Transaction: model.Transaction{
			Amount:       9505.97,
			Installments: 10,
			RequestedAt:  time.Date(2026, time.March, 14, 5, 15, 12, 0, time.UTC),
		},
		Customer: model.Customer{
			AvgAmount:      81.28,
			TxCount24h:     20,
			KnownMerchants: []string{"MERC-008", "MERC-007", "MERC-005"},
		},
		Merchant: model.Merchant{
			ID:        "MERC-068",
			MCC:       "7802",
			AvgAmount: 54.86,
		},
		Terminal: model.Terminal{
			CardPresent: true,
			KmFromHome:  952.27,
		},
	}

	got, err := New(testMetadata()).Vectorize(request)
	if err != nil {
		t.Fatalf("vectorize request: %v", err)
	}

	want := model.Vector{0.950597, 10.0 / 12.0, 1, 5.0 / 23.0, 5.0 / 6.0, -1, -1, 0.95227, 1, 0, 1, 1, 0.75, 0.005486}
	assertVectorClose(t, got, want)
}

func TestVectorizePreviousTransactionAndClamp(t *testing.T) {
	request := model.FraudRequest{
		Transaction: model.Transaction{
			Amount:       20000,
			Installments: 24,
			RequestedAt:  time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC),
		},
		Customer: model.Customer{
			AvgAmount:      100,
			TxCount24h:     -1,
			KnownMerchants: nil,
		},
		Merchant: model.Merchant{ID: "new", MCC: "unknown", AvgAmount: 20000},
		Terminal: model.Terminal{KmFromHome: -5},
		LastTransaction: &model.LastTransaction{
			Timestamp:     time.Date(2026, time.March, 8, 22, 30, 0, 0, time.UTC),
			KmFromCurrent: 2000,
		},
	}

	got, err := New(testMetadata()).Vectorize(request)
	if err != nil {
		t.Fatalf("vectorize request: %v", err)
	}

	want := model.Vector{1, 1, 1, 0, 0, 90.0 / 1440.0, 1, 0, 0, 0, 0, 1, 0.5, 1}
	assertVectorClose(t, got, want)
}

func TestVectorizeRejectsZeroCustomerAverage(t *testing.T) {
	request := model.FraudRequest{
		Transaction: model.Transaction{Amount: 1},
		Customer:    model.Customer{AvgAmount: 0},
	}
	if _, err := New(testMetadata()).Vectorize(request); err == nil {
		t.Fatal("vectorize with zero customer average succeeded, want error")
	}
}

func TestVectorizeRejectsInvalidNormalization(t *testing.T) {
	metadata := testMetadata()
	metadata.Normalization.MaxAmount = 0
	if _, err := New(metadata).Vectorize(model.FraudRequest{}); err == nil {
		t.Fatal("vectorize with invalid normalization succeeded, want error")
	}
}

func testMetadata() dataset.Metadata {
	return dataset.Metadata{
		MCCRisk: map[string]float64{
			"5411": 0.15,
			"7802": 0.75,
		},
		Normalization: dataset.Normalization{
			MaxAmount:            10000,
			MaxInstallments:      12,
			AmountVsAvgRatio:     10,
			MaxMinutes:           1440,
			MaxKM:                1000,
			MaxTxCount24h:        20,
			MaxMerchantAvgAmount: 10000,
		},
	}
}

func assertVectorClose(t *testing.T, got, want model.Vector) {
	t.Helper()
	const tolerance = 1e-9
	for dimension := range got {
		if math.Abs(got[dimension]-want[dimension]) > tolerance {
			t.Errorf("dimension %d = %.12f, want %.12f", dimension, got[dimension], want[dimension])
		}
	}
}
