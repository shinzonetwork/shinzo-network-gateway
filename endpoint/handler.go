package endpoint

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/goccy/go-json"
	"go.uber.org/zap"

	"github.com/shinzonetwork/shinzo-network-gateway/host"
)

// HostsSelector interface allows handler to get hosts for given collections.
type HostsSelector interface {
	SelectHosts(ctx context.Context, collections []string) ([]host.Host, error)
}

// Handler is a HTTP handler for "POST /graphql" endpoint.
type Handler struct {
	extractor CollectionsExtractor
	selector  HostsSelector
	logger    *zap.Logger
}

// graphQLRequest is the JSON body of a GraphQL-over-HTTP POST request per the GraphQL-over-HTTP spec.
// Query is required; OperationName, Variables, and Extensions are optional.
type graphQLRequest struct {
	Query         string          `json:"query"`
	OperationName string          `json:"operationName,omitempty"`
	Variables     json.RawMessage `json:"variables,omitempty"`
	Extensions    json.RawMessage `json:"extensions,omitempty"`
}

// graphQLResponse is the top-level GraphQL response per the GraphQL spec (Section 7).
// For execution results, Data is present (possibly null). For request errors, Data must be absent.
type graphQLResponse struct {
	Data       json.RawMessage `json:"data,omitempty"`
	Errors     []gqlError      `json:"errors,omitempty"`
	Extensions json.RawMessage `json:"extensions,omitempty"`
}

type hostResponse struct {
	response []byte
	err      error
}

var _ http.Handler = &Handler{}

// NewHandler creates new Handler instance.
func NewHandler(extractor CollectionsExtractor, selector HostsSelector, logger *zap.Logger) *Handler {
	return &Handler{
		extractor: extractor,
		selector:  selector,
		logger:    logger.Named("handler"),
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.logger.Sugar().Debugw("serving HTTP request", "from", r.RemoteAddr)

	contentType := h.getContentType(r)
	if contentType == "" {
		w.WriteHeader(http.StatusNotAcceptable)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error(), contentType)
		return
	}

	var gqlReq graphQLRequest
	if err := json.Unmarshal(body, &gqlReq); err != nil {
		h.writeError(w, requestErrorStatus(contentType), "invalid JSON body", contentType)
		return
	}

	collections, err := h.extractor.ExtractCollections(gqlReq.Query)
	if err != nil {
		h.writeError(w, requestErrorStatus(contentType), err.Error(), contentType)
		return
	}

	hosts, err := h.selector.SelectHosts(r.Context(), collections)
	if err != nil {
		h.writeError(w, http.StatusServiceUnavailable, err.Error(), contentType)
		return
	}

	responses := h.getHostsReponses(r.Context(), hosts, body)

	h.composeResponse(w, responses)
}

func (h *Handler) getHostsReponses(ctx context.Context, hosts []host.Host, body []byte) []hostResponse {
	responses := make([]hostResponse, len(hosts))
	wg := &sync.WaitGroup{}
	for i, host := range hosts {
		wg.Go(func() {
			responses[i] = h.queryHost(ctx, host, body)
		})
	}
	wg.Wait()

	return responses
}

func (h *Handler) queryHost(ctx context.Context, host host.Host, body []byte) hostResponse {
	panic("implement me!")
}

func (h *Handler) composeResponse(w http.ResponseWriter, responses []hostResponse) {
	panic("implement me!")
}

// getContentType picks the response content type from the Accept header.
// As defined in GraphQL-over-HTTP spec: prefers application/graphql-response+json; falls back to application/json.
// Returns "" if no supported type is acceptable (caller should respond 406).
func (h *Handler) getContentType(r *http.Request) string {
	accept := r.Header.Get("Accept")
	switch {
	case accept == "", strings.Contains(accept, "*/*"), strings.Contains(accept, contentTypeGraphQLResponse):
		return contentTypeGraphQLResponse
	case strings.Contains(accept, contentTypeJSON):
		return contentTypeJSON
	default:
		return ""
	}
}

// writeError writes a GraphQL error response with the given status code and message.
func (h *Handler) writeError(w http.ResponseWriter, status int, message string, mediaType string) {
	body, err := json.Marshal(graphQLResponse{Errors: []gqlError{{Message: message}}})
	if err != nil {
		h.logger.Sugar().Errorw("failed to marshal error response", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", mediaType+"; charset=utf-8")
	w.WriteHeader(status)
	_, err = w.Write(body)
	if err != nil {
		h.logger.Sugar().Errorw("failed to write error response", "error", err)
	}
}
