package endpoint

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

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

var _ http.Handler = &Handler{}

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

	contentType := getContentType(r)
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

// timeout defines timeout for host query
// TODO(tzdybal): extract timeout as config.
const timeout = 5 * time.Second

func (h *Handler) queryHost(ctx context.Context, host host.Host, body []byte) hostResponse {
	h.logger.Sugar().Debugw("sending query to host", "host", host, "body", body)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	url := string(host) + "/graphql"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return hostResponse{err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", contentTypeGraphQLResponse)

	// TODO(tzdybal): HTTP client per host, for more optimal resource usage
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return hostResponse{err: err}
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			h.logger.Sugar().Errorw("failed to close response body", "error", err)
		}
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return hostResponse{err: err}
	}

	return hostResponse{response: respBody}
}

type consensusLevel string

const (
	consensusFull    consensusLevel = "full"
	consensusPartial consensusLevel = "partial"
	consensusNone    consensusLevel = "none"
)

type responseGroup struct {
	Response json.RawMessage `json:"response"`
	Hosts    []host.Host     `json:"hosts"`
}

type consensusExtension struct {
	Consensus consensusLevel  `json:"consensus"`
	Responses []responseGroup `json:"responses,omitempty"`
}

func (h *Handler) composeResponse(w http.ResponseWriter, responses []hostResponse) {
	groups := groupResponses(responses)
	if len(groups) == 0 {
		h.writeError(w, http.StatusBadGateway, "no successful hosts responses", contentTypeGraphQLResponse)
		return
	}

	consensus := consensusFull
	if len(groups) > 1 {
		if len(groups[0].hosts) == len(groups[1].hosts) {
			consensus = consensusNone
		} else {
			consensus = consensusPartial
		}
	}

	ext := consensusExtension{Consensus: consensus}
	if consensus != consensusFull {
		ext.Responses = make([]responseGroup, len(groups))
		for i, g := range groups {
			ext.Responses[i] = responseGroup{
				Response: json.RawMessage(g.body),
				Hosts:    g.hosts,
			}
		}
	}

	extJSON, err := json.Marshal(ext)
	if err != nil {
		h.logger.Sugar().Errorw("failed to marshal consensus extension", "error", err)
		h.writeError(w, http.StatusInternalServerError, "internal error", contentTypeGraphQLResponse)
		return
	}

	resp := graphQLResponse{
		Data:       json.RawMessage(groups[0].body),
		Extensions: extJSON,
	}

	body, err := json.Marshal(resp)
	if err != nil {
		h.logger.Sugar().Errorw("failed to marshal response", "error", err)
		h.writeError(w, http.StatusInternalServerError, "internal error", contentTypeGraphQLResponse)
		return
	}

	w.Header().Set("Content-Type", contentTypeGraphQLResponse+"; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(body)
	if err != nil {
		h.logger.Sugar().Errorw("failed to write response", "error", err)
	}
}

type groupedResponse struct {
	body  string
	hosts []host.Host
}

func groupResponses(responses []hostResponse) []groupedResponse {
	order := make([]string, 0)
	groups := make(map[string]*groupedResponse)

	for i, r := range responses {
		if r.err != nil {
			continue
		}
		key := string(r.response)
		g, ok := groups[key]
		if !ok {
			g = &groupedResponse{body: key}
			groups[key] = g
			order = append(order, key)
		}
		g.hosts = append(g.hosts, host.Host("host-"+strconv.Itoa(i)))
	}

	// Sort by count descending, stable to preserve first-seen order for ties.
	sorted := make([]groupedResponse, len(order))
	for i, key := range order {
		sorted[i] = *groups[key]
	}
	slices.SortStableFunc(sorted, func(a, b groupedResponse) int {
		return len(b.hosts) - len(a.hosts)
	})

	return sorted
}

// getContentType picks the response content type from the Accept header.
// As defined in GraphQL-over-HTTP spec: prefers application/graphql-response+json; falls back to application/json.
// Returns "" if no supported type is acceptable (caller should respond 406).
func getContentType(r *http.Request) string {
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
