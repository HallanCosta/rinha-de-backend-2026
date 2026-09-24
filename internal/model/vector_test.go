package model

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestReferenceUnmarshal(t *testing.T) {
	data := []byte(`{
        "vector": [0.01, 0.0833, 0.05, 0.8261, 0.1667, -1, -1, 0.0432, 0.25, 0, 1, 0, 0.2, 0.0416],
        "label": "legit"
    }`)

	var reference Reference
	if err := json.Unmarshal(data, &reference); err != nil {
		t.Fatalf("unmarshal reference: %v", err)
	}

	if reference.Label != LabelLegit {
		t.Errorf("label = %q, want %q", reference.Label, LabelLegit)
	}
	if reference.Vector[5] != -1 || reference.Vector[6] != -1 {
		t.Errorf("sentinel dimensions = [%v %v], want [-1 -1]", reference.Vector[5], reference.Vector[6])
	}
	if reference.Vector[0] != 0.01 || reference.Vector[13] != 0.0416 {
		t.Errorf("vector endpoints = [%v %v], want [0.01 0.0416]", reference.Vector[0], reference.Vector[13])
	}
}

func TestVectorJSONRoundTrip(t *testing.T) {
	want := Vector{0.0041, 0.1667, 0.05, 0.7826, 0.3333, -1, -1, 0.0292, 0.15, 0, 1, 0, 0.15, 0.006}

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal vector: %v", err)
	}

	var got Vector
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal vector: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round-trip vector = %#v, want %#v", got, want)
	}
}

func TestVectorRejectsWrongDimensionCount(t *testing.T) {
	for name, data := range map[string][]byte{
		"too few":  []byte(`[0, 1, 2]`),
		"too many": []byte(`[0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14]`),
		"null":     []byte(`null`),
	} {
		t.Run(name, func(t *testing.T) {
			var vector Vector
			if err := json.Unmarshal(data, &vector); err == nil {
				t.Fatal("unmarshal succeeded, want dimension validation error")
			}
		})
	}
}

func TestReferenceRejectsUnsupportedLabel(t *testing.T) {
	data := []byte(`{
        "vector": [0, 0, 0, 0, 0, -1, -1, 0, 0, 0, 0, 0, 0, 0],
        "label": "unknown"
    }`)

	var reference Reference
	if err := json.Unmarshal(data, &reference); err == nil {
		t.Fatal("unmarshal unsupported label succeeded, want error")
	}
}

func TestNeighborIsFraud(t *testing.T) {
	if !(Neighbor{Label: LabelFraud}).IsFraud() {
		t.Error("fraud neighbor reported as legitimate")
	}
	if (Neighbor{Label: LabelLegit}).IsFraud() {
		t.Error("legit neighbor reported as fraud")
	}
}
