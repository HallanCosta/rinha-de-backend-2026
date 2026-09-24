package dataset

import (
	"context"
	"path/filepath"
	"testing"
)

// BenchmarkLoadOfficialReferences mede o custo de startup do loader com o
// arquivo oficial. Use -benchtime=1x: repetir o carregamento não representa o
// comportamento da API, que carrega o dataset uma vez por processo.
func BenchmarkLoadOfficialReferences(b *testing.B) {
	path := filepath.Join("..", "..", "resources", "references.json.gz")
	for index := 0; index < b.N; index++ {
		store, err := LoadReferences(context.Background(), path)
		if err != nil {
			b.Fatalf("load official references: %v", err)
		}
		b.ReportMetric(float64(store.Len()), "references")
	}
}
