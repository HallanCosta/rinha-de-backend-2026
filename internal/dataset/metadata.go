package dataset

import (
	"encoding/json"
	"fmt"
	"os"
)

// normalizationFile é o formato bruto do JSON de normalização.
//
// Os ponteiros permitem diferenciar um campo ausente de um campo presente com
// valor zero. Essa distinção é útil porque todos os divisores precisam ser
// positivos e o loader deve produzir um erro claro para arquivo incompleto.
// As tags convertem os nomes snake_case do arquivo para campos idiomáticos Go.
type normalizationFile struct {
	MaxAmount            *float64 `json:"max_amount"`
	MaxInstallments      *float64 `json:"max_installments"`
	AmountVsAvgRatio     *float64 `json:"amount_vs_avg_ratio"`
	MaxMinutes           *float64 `json:"max_minutes"`
	MaxKM                *float64 `json:"max_km"`
	MaxTxCount24h        *float64 `json:"max_tx_count_24h"`
	MaxMerchantAvgAmount *float64 `json:"max_merchant_avg_amount"`
}

// LoadMetadata carrega os dois arquivos JSON pequenos usados na futura
// vetorização.
func LoadMetadata(mccRiskPath, normalizationPath string) (Metadata, error) {
	var raw normalizationFile
	if err := decodeJSONFile(normalizationPath, &raw); err != nil {
		return Metadata{}, fmt.Errorf("load normalization metadata: %w", err)
	}
	normalization, err := buildNormalization(raw)
	if err != nil {
		return Metadata{}, err
	}

	// O map representa MCC -> risco e é equivalente a Record<string, number>.
	var mccRisk map[string]float64
	if err := decodeJSONFile(mccRiskPath, &mccRisk); err != nil {
		return Metadata{}, fmt.Errorf("load MCC risk metadata: %w", err)
	}
	if mccRisk == nil {
		return Metadata{}, fmt.Errorf("load MCC risk metadata: expected a JSON object")
	}
	for mcc, risk := range mccRisk {
		// Validamos na borda para que as próximas camadas possam confiar no
		// intervalo documentado, sem repetir essa checagem a cada requisição.
		if risk < 0 || risk > 1 {
			return Metadata{}, fmt.Errorf("load MCC risk metadata: %q has risk %v outside [0, 1]", mcc, risk)
		}
	}

	return Metadata{
		MCCRisk:       mccRisk,
		Normalization: normalization,
	}, nil
}

// buildNormalization valida todos os divisores antes de convertê-los para o
// tipo público. A slice de structs anônimas evita sete blocos de validação
// praticamente iguais e mantém a mensagem associada a cada chave JSON.
func buildNormalization(raw normalizationFile) (Normalization, error) {
	values := []struct {
		name  string
		value *float64
	}{
		{"max_amount", raw.MaxAmount},
		{"max_installments", raw.MaxInstallments},
		{"amount_vs_avg_ratio", raw.AmountVsAvgRatio},
		{"max_minutes", raw.MaxMinutes},
		{"max_km", raw.MaxKM},
		{"max_tx_count_24h", raw.MaxTxCount24h},
		{"max_merchant_avg_amount", raw.MaxMerchantAvgAmount},
	}
	for _, item := range values {
		if item.value == nil {
			return Normalization{}, fmt.Errorf("load normalization metadata: missing %q", item.name)
		}
		if *item.value <= 0 {
			return Normalization{}, fmt.Errorf("load normalization metadata: %q must be positive", item.name)
		}
	}

	return Normalization{
		MaxAmount:            *raw.MaxAmount,
		MaxInstallments:      *raw.MaxInstallments,
		AmountVsAvgRatio:     *raw.AmountVsAvgRatio,
		MaxMinutes:           *raw.MaxMinutes,
		MaxKM:                *raw.MaxKM,
		MaxTxCount24h:        *raw.MaxTxCount24h,
		MaxMerchantAvgAmount: *raw.MaxMerchantAvgAmount,
	}, nil
}

// decodeJSONFile recebe o destino como any (a interface vazia moderna de Go),
// permitindo reutilizar o mesmo leitor com structs e maps diferentes. O
// chamador ainda controla o tipo concreto e o decoder preenche esse valor por
// ponteiro, como uma função genérica de parsing tipado.
func decodeJSONFile(path string, destination any) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %q: %w", path, err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode %q: %w", path, err)
	}
	if err := rejectTrailingJSON(decoder); err != nil {
		return fmt.Errorf("decode %q: %w", path, err)
	}
	return nil
}
