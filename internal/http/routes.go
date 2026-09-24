package httpapi

import (
	"net/http"
	"sync/atomic"
)

// Readiness guarda o estado que o endpoint /ready expõe sem mutex manual.
// atomic.Bool é seguro quando o estado muda durante um startup controlado.
type Readiness struct {
	ready atomic.Bool
}

// NewReadiness cria um estado de prontidão com o valor inicial informado.
func NewReadiness(initial bool) *Readiness {
	readiness := &Readiness{}
	readiness.ready.Store(initial)
	return readiness
}

// SetReady publica a conclusão (ou revogação) do startup.
func (r *Readiness) SetReady(value bool) {
	if r != nil {
		r.ready.Store(value)
	}
}

// Ready retorna o estado atual de prontidão.
func (r *Readiness) Ready() bool {
	return r != nil && r.ready.Load()
}

// NewRouter registra exatamente os dois endpoints exigidos pelo desafio.
func NewRouter(scorer Scorer, readiness *Readiness, instance string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /fraud-score", NewHandler(scorer, instance))
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, _ *http.Request) {
		if readiness == nil || !readiness.Ready() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})
	return mux
}
