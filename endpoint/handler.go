package endpoint

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
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
	SelectHosts(ctx context.Context, n int, collections []string) ([]host.Host, error)
}

// Handler is a HTTP handler for "POST /graphql" endpoint.
type Handler struct {
	validators []Validator
	extractor  CollectionsExtractor
	selector   HostsSelector
	client     *http.Client
	logger     *zap.Logger

	defaultSampleSize int
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
	host     host.Host
	response []byte
	err      error
}

// extensions is a copy of Extensions from https://github.com/shinzonetwork/shinzo-querysig/blob/main/billing/
// The entire dependency tree of shinozo-querysig is to huge to import.
type extensions struct {
	RequestSignature string `json:"request_signature"`
	Nonce            string `json:"nonce"`
	QueryHash        string `json:"query_hash"`
	RequestTimestamp uint64 `json:"request_timestamp"`
	Fanout           int    `json:"fanout"`
}

// NewHandler creates new Handler instance.
func NewHandler(validators []Validator, extractor CollectionsExtractor, selector HostsSelector, defaultSampleSize int, logger *zap.Logger) *Handler {
	transport := &http.Transport{
		MaxIdleConnsPerHost:   maxIdleConnsPerHost,
		ResponseHeaderTimeout: responseHeaderTimeout,
	}

	return &Handler{
		validators:        validators,
		extractor:         extractor,
		selector:          selector,
		client:            &http.Client{Transport: transport},
		logger:            logger.Named("handler"),
		defaultSampleSize: defaultSampleSize,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("handling GraphQL request", zap.String("remote", r.RemoteAddr))

	contentType := getContentType(r)
	if contentType == "" {
		w.WriteHeader(http.StatusNotAcceptable)
		return
	}

	reqContentType, _, _ := strings.Cut(r.Header.Get("Content-Type"), ";")
	if strings.TrimSpace(reqContentType) != contentTypeJSON {
		h.writeError(w, http.StatusUnsupportedMediaType, "unsupported Content-Type", contentType)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		status := http.StatusBadRequest
		if errors.As(err, new(*http.MaxBytesError)) {
			status = http.StatusRequestEntityTooLarge
		}
		h.writeError(w, status, err.Error(), contentType)
		return
	}

	var gqlReq graphQLRequest
	if err := json.Unmarshal(body, &gqlReq); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body", contentType)
		return
	}

	query, err := parseQuery(gqlReq.Query)
	if err != nil {
		h.writeError(w, requestErrorStatus(contentType), err.Error(), contentType)
		return
	}

	validationReq := &ValidationRequest{Query: query, Header: r.Header}
	for _, v := range h.validators {
		if err := v.Validate(validationReq); err != nil {
			h.writeError(w, requestErrorStatus(contentType), fmt.Errorf("%w: %w", ErrValidation, err).Error(), contentType)
			return
		}
	}

	collections, err := h.extractor.ExtractCollections(query)
	if err != nil {
		h.writeError(w, requestErrorStatus(contentType), err.Error(), contentType)
		return
	}

	n, err := h.getFanout(gqlReq.Extensions)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid extensions", contentType)
		return
	}
	h.logger.Debug("selecting hosts", zap.Strings("collections", collections), zap.Int("fanout", n))
	hosts, err := h.selector.SelectHosts(r.Context(), n, collections)
	if err != nil {
		h.logger.Warn("failed to select hosts", zap.Strings("collections", collections), zap.Error(err))
		h.writeError(w, http.StatusServiceUnavailable, err.Error(), contentType)
		return
	}

	responses := h.getHostsResponses(r.Context(), hosts, body, r.Host, r.Header.Get("Authorization"))

	h.composeResponse(w, responses, contentType)
}

func (h *Handler) getFanout(ext json.RawMessage) (int, error) {
	fanout := h.defaultSampleSize
	if ext != nil {
		var deserialized extensions
		err := json.Unmarshal(ext, &deserialized)
		if err != nil {
			return 0, err
		}
		if deserialized.Fanout > 0 {
			fanout = deserialized.Fanout
		}
	}
	return fanout, nil
}

func (h *Handler) getHostsResponses(ctx context.Context, hosts []host.Host, body []byte, hostHeader, authHeader string) []hostResponse {
	responses := make([]hostResponse, len(hosts))
	wg := &sync.WaitGroup{}
	for i, host := range hosts {
		wg.Go(func() {
			responses[i] = h.queryHost(ctx, host, body, hostHeader, authHeader)
		})
	}
	wg.Wait()

	for i := range responses {
		responses[i].host = hosts[i]
	}

	return responses
}

// TODO(tzdybal): extract as config.
const (
	timeout               = 120 * time.Second
	maxIdleConnsPerHost   = 20
	responseHeaderTimeout = 10 * time.Second
	maxRequestBodySize    = 64 << 10 // 64 KiB
	maxResponseBodySize   = 1 << 20  // 1 MiB
)

func (h *Handler) queryHost(ctx context.Context, host host.Host, body []byte, hostHeader, authHeader string) hostResponse {
	logger := h.logger.With(zap.String("host", string(host)))
	logger.Debug("querying host", zap.Int("bodySize", len(body)))

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	url := string(host) + "/api/v0/graphql"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		logger.Warn("failed to build host request", zap.Error(err))
		return hostResponse{err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", contentTypeGraphQLResponse)

	// Forward original Host and Authorization headers so Shinzo Hosts can verify JWT audience against r.Host.
	if hostHeader != "" {
		req.Host = hostHeader
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	start := time.Now()
	resp, err := h.client.Do(req)
	if err != nil {
		logger.Warn("host query failed", zap.Duration("duration", time.Since(start)), zap.Error(err))
		return hostResponse{err: err}
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Warn("failed to close host response body", zap.Error(err))
		}
	}()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize+1))
	duration := time.Since(start)
	if err != nil {
		logger.Warn("failed to read host response", zap.Duration("duration", duration), zap.Error(err))
		return hostResponse{err: err}
	}

	if len(respBody) > maxResponseBodySize {
		logger.Warn("host response too large", zap.Int("maxSize", maxResponseBodySize))
		return hostResponse{err: fmt.Errorf("%w: host %s", ErrResponseTooLarge, host)}
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		logger.Warn("host returned error status", zap.Int("status", resp.StatusCode), zap.Duration("duration", duration))
		return hostResponse{err: fmt.Errorf("%w: %d from host %s", ErrUnexpectedStatus, resp.StatusCode, host)}
	}

	logger.Debug("host query succeeded",
		zap.Int("status", resp.StatusCode), zap.Int("responseSize", len(respBody)), zap.Duration("duration", duration))
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

func (h *Handler) composeResponse(w http.ResponseWriter, responses []hostResponse, contentType string) {
	groups := groupResponses(responses)
	if len(groups) == 0 {
		h.logger.Error("all host queries failed", zap.Int("hosts", len(responses)))
		h.writeError(w, http.StatusBadGateway, "no successful host responses", contentType)
		return
	}

	consensus := consensusFull
	if len(groups) > 1 {
		if len(groups[0].hosts) == len(groups[1].hosts) {
			consensus = consensusNone
		} else {
			consensus = consensusPartial
		}
		h.logger.Warn("hosts returned divergent responses",
			zap.String("consensus", string(consensus)), zap.Int("groups", len(groups)))
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
		h.logger.Error("failed to marshal consensus extension", zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "internal error", contentType)
		return
	}

	var hostResp graphQLResponse
	if err := json.Unmarshal([]byte(groups[0].body), &hostResp); err != nil {
		h.logger.Error("failed to parse host response", zap.Error(err))
		h.writeError(w, http.StatusBadGateway, "invalid host response", contentType)
		return
	}

	resp := graphQLResponse{
		Data:       hostResp.Data,
		Errors:     hostResp.Errors,
		Extensions: extJSON,
	}

	body, err := json.Marshal(resp)
	if err != nil {
		h.logger.Error("failed to marshal response", zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "internal error", contentType)
		return
	}

	w.Header().Set("Content-Type", contentType+"; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(body)
	if err != nil {
		h.logger.Warn("failed to write response", zap.Error(err))
	}
}

type groupedResponse struct {
	body  string
	hosts []host.Host
}

func groupResponses(responses []hostResponse) []groupedResponse {
	order := make([]string, 0)
	groups := make(map[string]*groupedResponse)

	for _, r := range responses {
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
		g.hosts = append(g.hosts, r.host)
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

// getContentType negotiates the response content type from the Accept header per RFC 9110.
// Returns "" if no supported type is acceptable (caller should respond 406).
func getContentType(r *http.Request) string {
	accept := r.Header.Get("Accept")
	if accept == "" {
		return contentTypeGraphQLResponse
	}

	return negotiateContentType(accept, []string{contentTypeGraphQLResponse, contentTypeJSON})
}

func negotiateContentType(accept string, supported []string) string {
	type entry struct {
		mtype, subtype string
		quality        float64
	}

	var entries []entry
	for part := range strings.SplitSeq(accept, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		mediaType, rest, _ := strings.Cut(part, ";")
		mediaType = strings.TrimSpace(mediaType)

		q := 1.0
		for param := range strings.SplitSeq(rest, ";") {
			key, val, ok := strings.Cut(strings.TrimSpace(param), "=")
			if ok && strings.TrimSpace(key) == "q" {
				if parsed, err := strconv.ParseFloat(strings.TrimSpace(val), 64); err == nil {
					q = parsed
				}
			}
		}

		mtype, subtype, _ := strings.Cut(mediaType, "/")
		entries = append(entries, entry{mtype: mtype, subtype: subtype, quality: q})
	}

	wildcards := func(e entry) int {
		n := 0
		if e.mtype == "*" {
			n++
		}
		if e.subtype == "*" {
			n++
		}
		return n
	}

	// Sort by quality descending, then specificity descending. Stable preserves client order for ties.
	slices.SortStableFunc(entries, func(a, b entry) int {
		if c := cmp.Compare(b.quality, a.quality); c != 0 {
			return c
		}
		return cmp.Compare(wildcards(a), wildcards(b))
	})

	for _, e := range entries {
		if e.quality == 0 {
			continue
		}
		for _, s := range supported {
			sType, sSubtype, _ := strings.Cut(s, "/")
			if (e.mtype == "*" || e.mtype == sType) && (e.subtype == "*" || e.subtype == sSubtype) {
				return s
			}
		}
	}

	return ""
}

// writeError writes a GraphQL error response with the given status code and message.
func (h *Handler) writeError(w http.ResponseWriter, status int, message string, contentType string) {
	h.logger.Debug("request failed", zap.Int("status", status), zap.String("message", message))
	body, err := json.Marshal(graphQLResponse{Errors: []gqlError{{Message: message}}})
	if err != nil {
		h.logger.Error("failed to marshal error response", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType+"; charset=utf-8")
	w.WriteHeader(status)
	_, err = w.Write(body)
	if err != nil {
		h.logger.Warn("failed to write error response", zap.Error(err))
	}
}
