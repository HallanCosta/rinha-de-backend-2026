package model

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestFraudRequestUnmarshal(t *testing.T) {
	payload := []byte(`{
        "id": "tx-3576980410",
        "transaction": {
            "amount": 384.88,
            "installments": 3,
            "requested_at": "2026-03-11T20:23:35Z"
        },
        "customer": {
            "avg_amount": 769.76,
            "tx_count_24h": 3,
            "known_merchants": ["MERC-009", "MERC-001", "MERC-001"]
        },
        "merchant": {
            "id": "MERC-001",
            "mcc": "5912",
            "avg_amount": 298.95
        },
        "terminal": {
            "is_online": false,
            "card_present": true,
            "km_from_home": 13.7090520965
        },
        "last_transaction": {
            "timestamp": "2026-03-11T14:58:35Z",
            "km_from_current": 18.8626479774
        }
    }`)

	var request FraudRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}

	if request.ID != "tx-3576980410" {
		t.Errorf("ID = %q, want tx-3576980410", request.ID)
	}
	if request.Transaction.Installments != 3 {
		t.Errorf("installments = %d, want 3", request.Transaction.Installments)
	}
	if !request.Transaction.RequestedAt.Equal(time.Date(2026, time.March, 11, 20, 23, 35, 0, time.UTC)) {
		t.Errorf("requested_at = %s, want 2026-03-11T20:23:35Z", request.Transaction.RequestedAt)
	}
	if !reflect.DeepEqual(request.Customer.KnownMerchants, []string{"MERC-009", "MERC-001", "MERC-001"}) {
		t.Errorf("known_merchants = %#v, want documented order and duplicates", request.Customer.KnownMerchants)
	}
	if request.Merchant.MCC != "5912" {
		t.Errorf("MCC = %q, want 5912", request.Merchant.MCC)
	}
	if request.Terminal.CardPresent != true {
		t.Error("card_present = false, want true")
	}
	if request.LastTransaction == nil {
		t.Fatal("last_transaction = nil, want object")
	}
	if !request.LastTransaction.Timestamp.Equal(time.Date(2026, time.March, 11, 14, 58, 35, 0, time.UTC)) {
		t.Errorf("last_transaction.timestamp = %s, want documented timestamp", request.LastTransaction.Timestamp)
	}
}

func TestFraudRequestUnmarshalNullLastTransaction(t *testing.T) {
	payload := []byte(`{
        "id": "tx-first-payment",
        "transaction": {
            "amount": 41.12,
            "installments": 2,
            "requested_at": "2026-03-11T18:45:53Z"
        },
        "customer": {
            "avg_amount": 82.24,
            "tx_count_24h": 3,
            "known_merchants": ["MERC-003", "MERC-016"]
        },
        "merchant": {
            "id": "MERC-016",
            "mcc": "5411",
            "avg_amount": 60.25
        },
        "terminal": {
            "is_online": false,
            "card_present": true,
            "km_from_home": 29.23
        },
        "last_transaction": null
    }`)

	var request FraudRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		t.Fatalf("unmarshal request with null last_transaction: %v", err)
	}
	if request.LastTransaction != nil {
		t.Fatalf("last_transaction = %#v, want nil", request.LastTransaction)
	}
}

func TestFraudResponseMarshal(t *testing.T) {
	data, err := json.Marshal(FraudResponse{Approved: false, FraudScore: 1.0})
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	const want = `{"approved":false,"fraud_score":1}`
	if string(data) != want {
		t.Fatalf("response JSON = %s, want %s", data, want)
	}

	var decoded FraudResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if decoded != (FraudResponse{Approved: false, FraudScore: 1.0}) {
		t.Errorf("decoded response = %#v, want approved=false fraud_score=1", decoded)
	}
}

func TestFraudRequestRejectsInvalidTimestamp(t *testing.T) {
	payload := []byte(`{"transaction":{"requested_at":"not-a-timestamp"}}`)

	var request FraudRequest
	if err := json.Unmarshal(payload, &request); err == nil {
		t.Fatal("unmarshal invalid timestamp succeeded, want error")
	}
}
