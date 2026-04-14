package endpoint

import "net/http"

// Endpoint handles GraphQL over HTTP requests.
type Endpoint struct {
	mux *http.ServeMux
}

// New creates new endpoint.
func New(handler *Handler) (*Endpoint, error) {
	mux, err := setupMux(handler)
	if err != nil {
		return nil, err
	}
	return &Endpoint{
		mux: mux,
	}, nil
}

func setupMux(handler *Handler) (*http.ServeMux, error) {
	mux := http.NewServeMux()

	mux.Handle("POST /graphql", handler)

	return mux, nil
}
