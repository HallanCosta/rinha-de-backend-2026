package dataset

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"rinha-backend-2026/internal/model"
)

// Load lê metadados e referências a partir dos caminhos configurados. O
// chamador poderá injetar o Dataset retornado nas futuras camadas de regras e
// busca, mantendo o main.go apenas como bootstrap.
func Load(ctx context.Context, paths Paths) (*Dataset, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}

	metadata, err := LoadMetadata(paths.MCCRiskPath, paths.NormalizationPath)
	if err != nil {
		return nil, err
	}

	references, err := LoadReferences(ctx, paths.ReferencesPath)
	if err != nil {
		return nil, err
	}

	return &Dataset{
		References: references,
		Metadata:   metadata,
	}, nil
}

// LoadReferences lê o array JSON compactado em gzip um elemento por vez,
// conforme a documentação. Ele não faz vetorização nem busca.
func LoadReferences(ctx context.Context, path string) (*MemoryReferenceStore, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}

	// os.Open devolve um *os.File; defer garante o fechamento quando a função
	// sair, inclusive nos retornos antecipados por erro.
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open references %q: %w", path, err)
	}
	defer file.Close()

	// O gzip.Reader descomprime durante a leitura, sem criar uma segunda cópia
	// descompactada inteira do arquivo na memória.
	reader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("open gzip references %q: %w", path, err)
	}
	defer reader.Close()

	// Decoder consome o JSON como stream. Isso é importante para o dataset grande:
	// não precisamos decodificar o array completo em uma árvore intermediária.
	decoder := json.NewDecoder(reader)
	if err := expectArrayStart(decoder); err != nil {
		return nil, fmt.Errorf("decode references %q: %w", path, err)
	}

	// O store já codifica cada vetor em formato compacto. Assim, o loader não
	// acumula uma slice []model.Reference grande antes de compactá-la.
	store := &MemoryReferenceStore{}
	for decoder.More() {
		// A checagem dentro do loop permite cancelar um carregamento longo por
		// context.Context, equivalente conceitualmente a um AbortSignal.
		if err := contextError(ctx); err != nil {
			return nil, err
		}

		var reference model.Reference
		if err := decoder.Decode(&reference); err != nil {
			return nil, fmt.Errorf("decode reference in %q: %w", path, err)
		}
		if err := store.append(reference); err != nil {
			return nil, fmt.Errorf("validate reference in %q: %w", path, err)
		}
	}

	// Depois dos elementos, o próximo token precisa fechar o array com ']'.
	if token, err := decoder.Token(); err != nil {
		return nil, fmt.Errorf("close references array %q: %w", path, err)
	} else if delimiter, ok := token.(json.Delim); !ok || delimiter != ']' {
		return nil, fmt.Errorf("close references array %q: expected ]", path)
	}

	if err := rejectTrailingJSON(decoder); err != nil {
		return nil, fmt.Errorf("decode references %q: %w", path, err)
	}

	return store, nil
}

// expectArrayStart valida o primeiro token antes de iniciar o loop de itens.
func expectArrayStart(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok || delimiter != '[' {
		return fmt.Errorf("expected a JSON array")
	}
	return nil
}

// rejectTrailingJSON garante que o arquivo contém somente um valor JSON.
func rejectTrailingJSON(decoder *json.Decoder) error {
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("unexpected trailing data: %w", err)
	}
	return fmt.Errorf("unexpected trailing JSON value")
}

// contextError centraliza o tratamento do contexto opcional. Aceitar nil aqui
// mantém os helpers fáceis de usar em testes, embora context.Background() seja
// a escolha idiomática para chamadas reais sem cancelamento.
func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
