package endpoint

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// TODO(tzdybal): refactor as config.
const readHeaderTimeout = 5 * time.Second

// Endpoint handles GraphQL over HTTP requests.
type Endpoint struct {
	server *http.Server
}

// New creates new endpoint.
func New(addr string, handler *Handler) (*Endpoint, error) {
	mux, err := setupMux(handler)
	if err != nil {
		return nil, err
	}
	return &Endpoint{
		server: &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: readHeaderTimeout,
		},
	}, nil
}

// ListenAndServe starts serving HTTP requests and blocks untile the server is shut down.
// TODO(tzdybal): add TLS support.
func (e *Endpoint) ListenAndServe() error {
	err := e.server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Close gracefully stops the HTTP server.
func (e *Endpoint) Close(ctx context.Context) error {
	return e.server.Shutdown(ctx)
}

func setupMux(handler *Handler) (*http.ServeMux, error) {
	mux := http.NewServeMux()

	mux.Handle("POST /graphql", handler)

	return mux, nil
}
