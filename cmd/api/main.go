// O pacote main produz o executável da API de detecção de fraude.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"rinha-backend-2026/internal/dataset"
	httpapi "rinha-backend-2026/internal/http"
	"rinha-backend-2026/internal/rules"
	"rinha-backend-2026/internal/search"
	"rinha-backend-2026/internal/vectorizer"
)

const (
	defaultReferencesPath    = "resources/references.json.gz"
	defaultMCCRiskPath       = "resources/mcc_risk.json"
	defaultNormalizationPath = "resources/normalization.json"
	defaultListenAddress     = ":9999"
)

func main() {
	// O main é o composition root: lê configuração, cria implementações e
	// conecta interfaces. As fórmulas ficam nos pacotes internos especializados.
	paths := dataset.Paths{
		ReferencesPath:    envOrDefault("REFERENCES_PATH", defaultReferencesPath),
		MCCRiskPath:       envOrDefault("MCC_RISK_PATH", defaultMCCRiskPath),
		NormalizationPath: envOrDefault("NORMALIZATION_PATH", defaultNormalizationPath),
	}

	loaded, err := dataset.Load(context.Background(), paths)
	if err != nil {
		// Sem dataset a instância não deve responder como se estivesse pronta.
		log.Fatalf("load fraud dataset: %v", err)
	}

	vectorizerEngine := vectorizer.New(loaded.Metadata)
	searcher := search.NewBruteForce(loaded.References)
	scorer := rules.New(vectorizerEngine, searcher)

	instance := envOrDefault("API_INSTANCE", "api-local")
	readiness := httpapi.NewReadiness(false)
	server := &http.Server{
		Addr:              envOrDefault("LISTEN_ADDR", defaultListenAddress),
		Handler:           httpapi.NewRouter(scorer, readiness, instance),
		ReadHeaderTimeout: 2 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	// Todas as dependências foram carregadas e conectadas antes de anunciar
	// prontidão ao Nginx ou ao executor da Rinha.
	readiness.SetReady(true)
	log.Printf("%s listening on %s with %d references", instance, server.Addr, loaded.References.Len())

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()

	// O Docker envia SIGTERM durante o shutdown. Encerrar as conexões com um
	// prazo curto evita cortar respostas que já estão em andamento.
	stopped := make(chan os.Signal, 1)
	signal.Notify(stopped, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stopped)

	select {
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve HTTP: %v", err)
		}
	case received := <-stopped:
		log.Printf("received %s, shutting down", received)
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			log.Fatalf("shutdown HTTP server: %v", err)
		}
	}
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
