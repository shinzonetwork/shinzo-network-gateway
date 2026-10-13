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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
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
	metrics    handlerMetrics

	defaultSampleSize int
}

// Values of the result attribute of shinzo.gateway.requests and shinzo.gateway.request.duration.
const (
	resultSuccess       = "success"
	resultInvalid       = "invalid"        // the request was rejected before routing
	resultNoHosts       = "no_hosts"       // no hosts serve the requested collections
	resultUpstreamError = "upstream_error" // no usable host response
	resultError         = "error"          // internal gateway error
)

// Values of the reason attribute of shinzo.gateway.upstream.errors.
const (
	reasonRequest   = "request"   // the host request could not be built
	reasonTransport = "transport" // connection error or timeout
	reasonRead      = "read"      // the response body could not be read
	reasonTooLarge  = "too_large" // the response body exceeds the size limit
	reasonStatus    = "status"    // non-2xx HTTP status
)

type handlerMetrics struct {
	requests         metric.Int64Counter
	requestDuration  metric.Float64Histogram
	upstreamDuration metric.Float64Histogram
	upstreamErrors   metric.Int64Counter
	consensus        metric.Int64Counter

	results         map[string]metric.MeasurementOption
	upstreamSuccess metric.MeasurementOption
	upstreamFailure metric.MeasurementOption
	upstreamReasons map[string]metric.MeasurementOption
	consensusLevels map[consensusLevel]metric.MeasurementOption
}

func newHandlerMetrics() handlerMetrics {
	meter := otel.Meter("github.com/shinzonetwork/shinzo-network-gateway/endpoint")
	requests, err := meter.Int64Counter("shinzo.gateway.requests",
		metric.WithDescription("GraphQL requests handled by the gateway, by result."),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		otel.Handle(err)
	}
	requestDuration, err := meter.Float64Histogram("shinzo.gateway.request.duration",
		metric.WithDescription("Duration of handling one GraphQL request, by result."),
		metric.WithUnit("ms"),
	)
	if err != nil {
		otel.Handle(err)
	}
	upstreamDuration, err := meter.Float64Histogram("shinzo.gateway.upstream.duration",
		metric.WithDescription("Duration of one query sent to a host, by result."),
		metric.WithUnit("ms"),
	)
	if err != nil {
		otel.Handle(err)
	}
	upstreamErrors, err := meter.Int64Counter("shinzo.gateway.upstream.errors",
		metric.WithDescription("Failed queries to hosts, by reason. Queries canceled by the client are not counted."),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		otel.Handle(err)
	}
	consensus, err := meter.Int64Counter("shinzo.gateway.consensus",
		metric.WithDescription("Compared host responses, by consensus level (full, partial, none)."),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		otel.Handle(err)
	}

	m := handlerMetrics{
		requests:         requests,
		requestDuration:  requestDuration,
		upstreamDuration: upstreamDuration,
		upstreamErrors:   upstreamErrors,
		consensus:        consensus,
		results:          make(map[string]metric.MeasurementOption),
		upstreamSuccess:  metric.WithAttributeSet(attribute.NewSet(attribute.String("result", "success"))),
		upstreamFailure:  metric.WithAttributeSet(attribute.NewSet(attribute.String("result", "error"))),
		upstreamReasons:  make(map[string]metric.MeasurementOption),
		consensusLevels:  make(map[consensusLevel]metric.MeasurementOption),
	}
	for _, r := range []string{resultSuccess, resultInvalid, resultNoHosts, resultUpstreamError, resultError} {
		m.results[r] = metric.WithAttributeSet(attribute.NewSet(attribute.String("result", r)))
	}
	for _, r := range []string{reasonRequest, reasonTransport, reasonRead, reasonTooLarge, reasonStatus} {
		m.upstreamReasons[r] = metric.WithAttributeSet(attribute.NewSet(attribute.String("reason", r)))
	}
	for _, c := range []consensusLevel{consensusFull, consensusPartial, consensusNone} {
		m.consensusLevels[c] = metric.WithAttributeSet(attribute.NewSet(attribute.String("consensus", string(c))))
	}
	return m
}

// countUpstreamError counts a failed host query. Queries canceled by the client are an expected outcome.
func (h *Handler) countUpstreamError(ctx context.Context, reason string, err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	h.metrics.upstreamErrors.Add(ctx, 1, h.metrics.upstreamReasons[reason])
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
		metrics:           newHandlerMetrics(),
		defaultSampleSize: defaultSampleSize,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	result := h.serve(w, r)
	// a fresh context: the request context may already be canceled
	ctx := context.WithoutCancel(r.Context())
	h.metrics.requests.Add(ctx, 1, h.metrics.results[result])
	h.metrics.requestDuration.Record(ctx, float64(time.Since(start))/float64(time.Millisecond), h.metrics.results[result])
}

// serve handles the request and returns its result for metrics.
func (h *Handler) serve(w http.ResponseWriter, r *http.Request) string {
	h.logger.Debug("handling GraphQL request", zap.String("remote", r.RemoteAddr))

	contentType := getContentType(r)
	if contentType == "" {
		w.WriteHeader(http.StatusNotAcceptable)
		return resultInvalid
	}

	reqContentType, _, _ := strings.Cut(r.Header.Get("Content-Type"), ";")
	if strings.TrimSpace(reqContentType) != contentTypeJSON {
		h.writeError(w, http.StatusUnsupportedMediaType, "unsupported Content-Type", contentType)
		return resultInvalid
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		status := http.StatusBadRequest
		if errors.As(err, new(*http.MaxBytesError)) {
			status = http.StatusRequestEntityTooLarge
		}
		h.writeError(w, status, err.Error(), contentType)
		return resultInvalid
	}

	var gqlReq graphQLRequest
	if err := json.Unmarshal(body, &gqlReq); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body", contentType)
		return resultInvalid
	}

	query, err := parseQuery(gqlReq.Query)
	if err != nil {
		h.writeError(w, requestErrorStatus(contentType), err.Error(), contentType)
		return resultInvalid
	}

	validationReq := &ValidationRequest{Query: query, Header: r.Header}
	for _, v := range h.validators {
		if err := v.Validate(validationReq); err != nil {
			h.writeError(w, requestErrorStatus(contentType), fmt.Errorf("%w: %w", ErrValidation, err).Error(), contentType)
			return resultInvalid
		}
	}

	collections, err := h.extractor.ExtractCollections(query)
	if err != nil {
		h.writeError(w, requestErrorStatus(contentType), err.Error(), contentType)
		return resultInvalid
	}

	n, err := h.getFanout(gqlReq.Extensions)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid extensions", contentType)
		return resultInvalid
	}
	h.logger.Debug("selecting hosts", zap.Strings("collections", collections), zap.Int("fanout", n))
	hosts, err := h.selector.SelectHosts(r.Context(), n, collections)
	if err != nil {
		h.logger.Warn("failed to select hosts", zap.Strings("collections", collections), zap.Error(err))
		h.writeError(w, http.StatusServiceUnavailable, err.Error(), contentType)
		return resultNoHosts
	}

	responses := h.getHostsResponses(r.Context(), hosts, body, r.Host, r.Header.Get("Authorization"))

	return h.composeResponse(r.Context(), w, responses, contentType)
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
		h.countUpstreamError(ctx, reasonRequest, err)
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
		h.recordUpstream(ctx, start, reasonTransport, err)
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
		h.recordUpstream(ctx, start, reasonRead, err)
		return hostResponse{err: err}
	}

	if len(respBody) > maxResponseBodySize {
		logger.Warn("host response too large", zap.Int("maxSize", maxResponseBodySize))
		err := fmt.Errorf("%w: host %s", ErrResponseTooLarge, host)
		h.recordUpstream(ctx, start, reasonTooLarge, err)
		return hostResponse{err: err}
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		logger.Warn("host returned error status", zap.Int("status", resp.StatusCode), zap.Duration("duration", duration))
		err := fmt.Errorf("%w: %d from host %s", ErrUnexpectedStatus, resp.StatusCode, host)
		h.recordUpstream(ctx, start, reasonStatus, err)
		return hostResponse{err: err}
	}

	logger.Debug("host query succeeded",
		zap.Int("status", resp.StatusCode), zap.Int("responseSize", len(respBody)), zap.Duration("duration", duration))
	h.recordUpstream(ctx, start, "", nil)
	return hostResponse{response: respBody}
}

// recordUpstream records the duration of a host query sent at start and, if err is not nil, counts it by reason.
func (h *Handler) recordUpstream(ctx context.Context, start time.Time, reason string, err error) {
	outcome := h.metrics.upstreamSuccess
	if err != nil {
		outcome = h.metrics.upstreamFailure
		h.countUpstreamError(ctx, reason, err)
	}
	h.metrics.upstreamDuration.Record(ctx, float64(time.Since(start))/float64(time.Millisecond), outcome)
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

func (h *Handler) composeResponse(ctx context.Context, w http.ResponseWriter, responses []hostResponse, contentType string) string {
	groups := groupResponses(responses)
	if len(groups) == 0 {
		h.logger.Error("all host queries failed", zap.Int("hosts", len(responses)))
		h.writeError(w, http.StatusBadGateway, "no successful host responses", contentType)
		return resultUpstreamError
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
	h.metrics.consensus.Add(context.WithoutCancel(ctx), 1, h.metrics.consensusLevels[consensus])

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
		return resultError
	}

	var hostResp graphQLResponse
	if err := json.Unmarshal([]byte(groups[0].body), &hostResp); err != nil {
		h.logger.Error("failed to parse host response", zap.Error(err))
		h.writeError(w, http.StatusBadGateway, "invalid host response", contentType)
		return resultUpstreamError
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
		return resultError
	}

	w.Header().Set("Content-Type", contentType+"; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(body)
	if err != nil {
		h.logger.Warn("failed to write response", zap.Error(err))
	}
	return resultSuccess
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
