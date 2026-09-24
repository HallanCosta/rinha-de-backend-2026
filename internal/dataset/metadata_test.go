package dataset

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMetadataRejectsMissingNormalizationField(t *testing.T) {
	dir := t.TempDir()
	mccPath := filepath.Join(dir, "mcc_risk.json")
	normalizationPath := filepath.Join(dir, "normalization.json")

	writeFile(t, mccPath, `{}`)
	writeFile(t, normalizationPath, `{
        "max_amount": 10000,
        "max_installments": 12,
        "amount_vs_avg_ratio": 10,
        "max_minutes": 1440,
        "max_km": 1000,
        "max_tx_count_24h": 20
    }`)

	_, err := LoadMetadata(mccPath, normalizationPath)
	if err == nil {
		t.Fatal("load incomplete normalization succeeded, want error")
	}
	if !strings.Contains(err.Error(), "max_merchant_avg_amount") {
		t.Errorf("error = %v, want missing-field context", err)
	}
}

func TestLoadMetadataRejectsNonPositiveNormalization(t *testing.T) {
	dir := t.TempDir()
	mccPath := filepath.Join(dir, "mcc_risk.json")
	normalizationPath := filepath.Join(dir, "normalization.json")

	writeFile(t, mccPath, `{}`)
	writeFile(t, normalizationPath, `{
        "max_amount": 0,
        "max_installments": 12,
        "amount_vs_avg_ratio": 10,
        "max_minutes": 1440,
        "max_km": 1000,
        "max_tx_count_24h": 20,
        "max_merchant_avg_amount": 10000
    }`)

	_, err := LoadMetadata(mccPath, normalizationPath)
	if err == nil {
		t.Fatal("load zero normalization succeeded, want error")
	}
	if !strings.Contains(err.Error(), "max_amount") {
		t.Errorf("error = %v, want max_amount context", err)
	}
}

func TestLoadMetadataRejectsMCCRiskOutsideRange(t *testing.T) {
	dir := t.TempDir()
	mccPath := filepath.Join(dir, "mcc_risk.json")
	normalizationPath := filepath.Join(dir, "normalization.json")

	writeFile(t, mccPath, `{"5912":1.1}`)
	writeFile(t, normalizationPath, validNormalizationJSON())

	_, err := LoadMetadata(mccPath, normalizationPath)
	if err == nil {
		t.Fatal("load out-of-range MCC risk succeeded, want error")
	}
	if !strings.Contains(err.Error(), "5912") {
		t.Errorf("error = %v, want MCC context", err)
	}
}

func TestLoadMetadataRejectsTrailingJSON(t *testing.T) {
	dir := t.TempDir()
	mccPath := filepath.Join(dir, "mcc_risk.json")
	normalizationPath := filepath.Join(dir, "normalization.json")

	writeFile(t, mccPath, `{} {"extra":true}`)
	writeFile(t, normalizationPath, validNormalizationJSON())

	_, err := LoadMetadata(mccPath, normalizationPath)
	if err == nil {
		t.Fatal("load metadata with trailing JSON succeeded, want error")
	}
	if !strings.Contains(err.Error(), "trailing") {
		t.Errorf("error = %v, want trailing-data context", err)
	}
}
