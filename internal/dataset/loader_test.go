package dataset

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rinha-backend-2026/internal/model"
)

func TestLoadDataset(t *testing.T) {
	dir := t.TempDir()
	paths := writeValidDataset(t, dir)

	loaded, err := Load(context.Background(), paths)
	if err != nil {
		t.Fatalf("load dataset: %v", err)
	}

	if loaded.References.Len() != 2 {
		t.Fatalf("reference count = %d, want 2", loaded.References.Len())
	}

	var vector model.Vector
	label, ok := loaded.References.At(0, &vector)
	if !ok {
		t.Fatal("reference 0 not found")
	}
	if label != model.LabelLegit {
		t.Errorf("reference 0 label = %q, want %q", label, model.LabelLegit)
	}
	if vector[5] != -1 || vector[6] != -1 {
		t.Errorf("reference 0 sentinels = [%v %v], want [-1 -1]", vector[5], vector[6])
	}

	label, ok = loaded.References.At(1, nil)
	if !ok || label != model.LabelFraud {
		t.Errorf("reference 1 = (%q, %v), want (fraud, true)", label, ok)
	}
	if _, ok := loaded.References.At(2, nil); ok {
		errorf := "out-of-range reference reported as present"
		t.Error(errorf)
	}

	if got := loaded.Metadata.RiskFor("5912"); got != 0.2 {
		t.Errorf("known MCC risk = %v, want 0.2", got)
	}
	if got := loaded.Metadata.RiskFor("9999"); got != defaultMCCRisk {
		t.Errorf("unknown MCC risk = %v, want %v", got, defaultMCCRisk)
	}
	if loaded.Metadata.Normalization.MaxAmount != 10000 {
		t.Errorf("max_amount = %v, want 10000", loaded.Metadata.Normalization.MaxAmount)
	}
}

func TestLoadReferencesMissingFile(t *testing.T) {
	_, err := LoadReferences(context.Background(), filepath.Join(t.TempDir(), "missing.json.gz"))
	if err == nil {
		t.Fatal("load missing references succeeded, want error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error = %v, want os.ErrNotExist", err)
	}
}

func TestLoadReferencesRejectsCorruptGzip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "references.json.gz")
	if err := os.WriteFile(path, []byte("not gzip"), 0o600); err != nil {
		t.Fatalf("write corrupt gzip: %v", err)
	}

	_, err := LoadReferences(context.Background(), path)
	if err == nil {
		t.Fatal("load corrupt gzip succeeded, want error")
	}
	if !strings.Contains(err.Error(), "gzip") {
		t.Errorf("error = %v, want gzip context", err)
	}
}

func TestLoadReferencesRejectsNonArrayRoot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "references.json.gz")
	writeGzip(t, path, []byte(`{"vector":[]}`))

	_, err := LoadReferences(context.Background(), path)
	if err == nil {
		t.Fatal("load object root succeeded, want array error")
	}
	if !strings.Contains(err.Error(), "JSON array") {
		t.Errorf("error = %v, want JSON array context", err)
	}
}

func TestLoadReferencesRejectsInvalidReference(t *testing.T) {
	tests := map[string]string{
		"wrong vector dimensions": `[{"vector":[0,1],"label":"legit"}]`,
		"unknown label":           `[{"vector":[0,0,0,0,0,-1,-1,0,0,0,0,0,0,0],"label":"unknown"}]`,
		"invalid json":            `[{`,
	}

	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "references.json.gz")
			writeGzip(t, path, []byte(content))

			_, err := LoadReferences(context.Background(), path)
			if err == nil {
				t.Fatal("load invalid reference succeeded, want error")
			}
		})
	}
}

func TestLoadReferencesRejectsTrailingJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "references.json.gz")
	content := validReferencesJSON() + ` {"extra":true}`
	writeGzip(t, path, []byte(content))

	_, err := LoadReferences(context.Background(), path)
	if err == nil {
		t.Fatal("load with trailing JSON succeeded, want error")
	}
	if !strings.Contains(err.Error(), "trailing") {
		t.Errorf("error = %v, want trailing-data context", err)
	}
}

func writeValidDataset(t *testing.T, dir string) Paths {
	t.Helper()

	referencesPath := filepath.Join(dir, "references.json.gz")
	writeGzip(t, referencesPath, []byte(validReferencesJSON()))

	mccRiskPath := filepath.Join(dir, "mcc_risk.json")
	writeFile(t, mccRiskPath, `{"5912":0.2,"7802":0.75}`)

	normalizationPath := filepath.Join(dir, "normalization.json")
	writeFile(t, normalizationPath, validNormalizationJSON())

	return Paths{
		ReferencesPath:    referencesPath,
		MCCRiskPath:       mccRiskPath,
		NormalizationPath: normalizationPath,
	}
}

func validReferencesJSON() string {
	references := []model.Reference{
		{Vector: model.Vector{0.01, 0.0833, 0.05, 0.8261, 0.1667, -1, -1, 0.0432, 0.25, 0, 1, 0, 0.2, 0.0416}, Label: model.LabelLegit},
		{Vector: model.Vector{0.5796, 0.9167, 1, 0.0435, 0, 0.0056, 0.4394, 0.4598, 0.4, 1, 0, 1, 0.85, 0.0032}, Label: model.LabelFraud},
	}
	data, err := json.Marshal(references)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func validNormalizationJSON() string {
	return `{
        "max_amount": 10000,
        "max_installments": 12,
        "amount_vs_avg_ratio": 10,
        "max_minutes": 1440,
        "max_km": 1000,
        "max_tx_count_24h": 20,
        "max_merchant_avg_amount": 10000
    }`
}

func writeGzip(t *testing.T, path string, content []byte) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create gzip file: %v", err)
	}
	writer := gzip.NewWriter(file)
	if _, err := writer.Write(content); err != nil {
		_ = file.Close()
		t.Fatalf("write gzip content: %v", err)
	}
	if err := writer.Close(); err != nil {
		_ = file.Close()
		t.Fatalf("close gzip writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close gzip file: %v", err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture %q: %v", path, err)
	}
}
