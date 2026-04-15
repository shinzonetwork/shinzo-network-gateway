package endpoint

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/shinzonetwork/shinzo-network-gateway/host"
)

var (
	errParse   = errors.New("parse error")
	errNoHosts = errors.New("no hosts")
	errFail    = errors.New("fail")
)

type mockExtractor struct {
	mock.Mock
}

func (m *mockExtractor) ExtractCollections(graphql string) ([]string, error) {
	args := m.Called(graphql)
	return args.Get(0).([]string), args.Error(1)
}

type mockSelector struct {
	mock.Mock
}

func (m *mockSelector) SelectHosts(ctx context.Context, collections []string) ([]host.Host, error) {
	args := m.Called(ctx, collections)
	return args.Get(0).([]host.Host), args.Error(1)
}

func TestGetContentType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		request  *http.Request
		expected string
	}{
		{
			name:     "empty header",
			request:  &http.Request{},
			expected: contentTypeGraphQLResponse,
		},
		{
			name:     "accept any",
			request:  &http.Request{Header: map[string][]string{"Accept": {"*/*"}}},
			expected: contentTypeGraphQLResponse,
		},
		{
			name:     "accept GraphQL response json",
			request:  &http.Request{Header: map[string][]string{"Accept": {contentTypeGraphQLResponse}}},
			expected: contentTypeGraphQLResponse,
		},
		{
			name:     "accept GraphQL response json with encoding",
			request:  &http.Request{Header: map[string][]string{"Accept": {contentTypeGraphQLResponse + "; charset=utf-8"}}},
			expected: contentTypeGraphQLResponse,
		},
		{
			name:     "accept application/json",
			request:  &http.Request{Header: map[string][]string{"Accept": {contentTypeJSON}}},
			expected: contentTypeJSON,
		},
		{
			name:     "accept text/html",
			request:  &http.Request{Header: map[string][]string{"Accept": {"text/html"}}},
			expected: "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := getContentType(c.request)
			require.Equal(t, c.expected, result)
		})
	}
}

func TestHandlerGetHostsResponses(t *testing.T) {
	t.Parallel()

	logger, _ := zap.NewDevelopment()

	body := []byte(`{"query":"{ hero { name } }"}`)

	cases := []struct {
		name    string
		kinds   []hostKind
		timeout time.Duration
	}{
		{
			name:  "1 ok host",
			kinds: []hostKind{kindOK},
		},
		{
			name:  "3 ok hosts",
			kinds: []hostKind{kindOK, kindOK, kindOK},
		},
		{
			name:    "2 ok hosts and timeout host",
			kinds:   []hostKind{kindOK, kindOK, kindTimeout},
			timeout: 100 * time.Millisecond,
		},
		{
			name:  "2 ok hosts and error host",
			kinds: []hostKind{kindOK, kindOK, kindError},
		},
		{
			name:    "2 ok hosts and unreachable host",
			kinds:   []hostKind{kindOK, kindOK, kindUnreachable},
			timeout: 100 * time.Millisecond,
		},
		{
			name:    "1 ok host, 1 timeout host and 1 error host",
			kinds:   []hostKind{kindOK, kindTimeout, kindError},
			timeout: 100 * time.Millisecond,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			th := setupTestHosts(c.kinds)
			defer th.cleanup()

			h := NewHandler(nil, nil, logger)
			require.NotNil(t, h)

			timeout := c.timeout
			if timeout == 0 {
				timeout = time.Second
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			responses := h.getHostsReponses(ctx, th.hosts, body)

			require.Len(t, responses, len(c.kinds))

			for i, resp := range responses {
				if th.wantErr[i] {
					assert.Error(t, resp.err, "response[%d] expected error", i)
					assert.Nil(t, resp.response, "response[%d] expected nil body", i)
				} else {
					assert.NoError(t, resp.err, "response[%d] unexpected error", i)
					assert.NotEmpty(t, resp.response, "response[%d] expected non-empty body", i)
				}

				// Every reachable host must have been hit exactly once.
				if c.kinds[i] != kindUnreachable {
					assert.Equal(t, int32(1), th.hits[i].Load(), "host[%d] expected exactly 1 hit", i)
				}
			}
		})
	}
}

func TestHandler(t *testing.T) {
	t.Parallel()

	logger, _ := zap.NewDevelopment()
	defer func() {
		_ = logger.Sync()
	}()

	cases := []struct {
		name           string
		body           string
		accept         string
		setupExtractor func(*mockExtractor)
		setupSelector  func(*mockSelector, []host.Host)
		wantStatus     int
		wantBodyHas    string
	}{
		{
			name:       "unsupported accept header",
			body:       `{"query":"{ hero { name } }"}`,
			accept:     "text/html",
			wantStatus: http.StatusNotAcceptable,
		},
		{
			name:        "invalid JSON body",
			body:        `not json`,
			wantStatus:  http.StatusBadRequest,
			wantBodyHas: "invalid JSON body",
		},
		{
			name: "extractor error",
			body: `{"query":"bad"}`,
			setupExtractor: func(ext *mockExtractor) {
				ext.On("ExtractCollections", "bad").Return([]string(nil), errParse)
			},
			wantStatus:  http.StatusBadRequest,
			wantBodyHas: "parse error",
		},
		{
			name: "selector error",
			body: `{"query":"{ hero { name } }"}`,
			setupExtractor: func(ext *mockExtractor) {
				ext.On("ExtractCollections", "{ hero { name } }").Return([]string{"hero"}, nil)
			},
			setupSelector: func(sel *mockSelector, _ []host.Host) {
				sel.On("SelectHosts", mock.Anything, []string{"hero"}).Return([]host.Host(nil), errNoHosts)
			},
			wantStatus:  http.StatusServiceUnavailable,
			wantBodyHas: "no hosts",
		},
		{
			name: "successful query",
			body: `{"query":"{ hero { name } }"}`,
			setupExtractor: func(ext *mockExtractor) {
				ext.On("ExtractCollections", "{ hero { name } }").Return([]string{"hero"}, nil)
			},
			setupSelector: func(sel *mockSelector, hosts []host.Host) {
				sel.On("SelectHosts", mock.Anything, []string{"hero"}).Return(hosts, nil)
			},
			wantStatus: http.StatusOK,
			wantBodyHas: `{"data":{"hero":{"name":"Luke"}},"extensions":{"consensus":"full"}}`,
		},
		{
			name:   "legacy content type uses 200 for request errors",
			body:   `{"query":"bad"}`,
			accept: "application/json",
			setupExtractor: func(ext *mockExtractor) {
				ext.On("ExtractCollections", "bad").Return([]string(nil), errParse)
			},
			wantStatus:  http.StatusOK,
			wantBodyHas: "parse error",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			okHost := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/graphql-response+json")
				_, _ = w.Write([]byte(`{"data":{"hero":{"name":"Luke"}}}`))
			}))
			defer okHost.Close()
			ext := &mockExtractor{}
			sel := &mockSelector{}
			if c.setupExtractor != nil {
				c.setupExtractor(ext)
			}
			if c.setupSelector != nil {
				c.setupSelector(sel, []host.Host{host.Host(okHost.URL)})
			}

			h := NewHandler(ext, sel, logger)

			accept := c.accept
			if accept == "" {
				accept = contentTypeGraphQLResponse
			}

			req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(c.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", accept)
			w := httptest.NewRecorder()

			h.ServeHTTP(w, req)

			assert.Equal(t, c.wantStatus, w.Code)
			if c.wantBodyHas != "" {
				assert.Contains(t, w.Body.String(), c.wantBodyHas)
			}

			ext.AssertExpectations(t)
			sel.AssertExpectations(t)
		})
	}
}

func TestGroupResponses(t *testing.T) {
	t.Parallel()

	dataA := []byte(`{"data":"a"}`)
	dataB := []byte(`{"data":"b"}`)
	dataC := []byte(`{"data":"c"}`)

	cases := []struct {
		name       string
		responses  []hostResponse
		wantLen    int
		wantBodies []string
		wantHosts  [][]host.Host
	}{
		{
			name:      "all errors returns empty",
			responses: []hostResponse{{err: errFail}, {err: errFail}},
			wantLen:   0,
		},
		{
			name: "single response",
			responses: []hostResponse{
				{response: dataA},
			},
			wantLen:    1,
			wantBodies: []string{string(dataA)},
			wantHosts:  [][]host.Host{{"host-0"}},
		},
		{
			name: "all same response - full consensus",
			responses: []hostResponse{
				{response: dataA},
				{response: dataA},
				{response: dataA},
			},
			wantLen:    1,
			wantBodies: []string{string(dataA)},
			wantHosts:  [][]host.Host{{"host-0", "host-1", "host-2"}},
		},
		{
			name: "majority wins - partial consensus",
			responses: []hostResponse{
				{response: dataA},
				{response: dataB},
				{response: dataA},
			},
			wantLen:    2,
			wantBodies: []string{string(dataA), string(dataB)},
			wantHosts:  [][]host.Host{{"host-0", "host-2"}, {"host-1"}},
		},
		{
			name: "tie - none consensus",
			responses: []hostResponse{
				{response: dataA},
				{response: dataB},
			},
			wantLen:    2,
			wantBodies: []string{string(dataA), string(dataB)},
			wantHosts:  [][]host.Host{{"host-0"}, {"host-1"}},
		},
		{
			name: "errors are skipped",
			responses: []hostResponse{
				{err: errFail},
				{response: dataA},
				{response: dataA},
			},
			wantLen:    1,
			wantBodies: []string{string(dataA)},
			wantHosts:  [][]host.Host{{"host-1", "host-2"}},
		},
		{
			name: "three distinct responses ordered by popularity",
			responses: []hostResponse{
				{response: dataC},
				{response: dataA},
				{response: dataA},
				{response: dataC},
				{response: dataA},
				{response: dataB},
			},
			wantLen:    3,
			wantBodies: []string{string(dataA), string(dataC), string(dataB)},
			wantHosts:  [][]host.Host{{"host-1", "host-2", "host-4"}, {"host-0", "host-3"}, {"host-5"}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			groups := groupResponses(c.responses)
			require.Len(t, groups, c.wantLen)

			if c.wantLen == 0 {
				return
			}

			for i, wantBody := range c.wantBodies {
				assert.Equal(t, wantBody, groups[i].body, "group[%d] body", i)
				assert.Equal(t, c.wantHosts[i], groups[i].hosts, "group[%d] hosts", i)
			}

			// Verify descending order by popularity.
			for i := 1; i < len(groups); i++ {
				assert.GreaterOrEqual(t, len(groups[i-1].hosts), len(groups[i].hosts),
					"groups should be ordered by popularity descending")
			}
		})
	}
}

type hostKind int

const (
	kindOK hostKind = iota
	kindTimeout
	kindError
	kindUnreachable
)

type testHosts struct {
	hosts   []host.Host
	hits    []atomic.Int32
	wantErr []bool
	cleanup func()
}

func setupTestHosts(kinds []hostKind) testHosts {
	th := testHosts{
		hosts:   make([]host.Host, len(kinds)),
		hits:    make([]atomic.Int32, len(kinds)),
		wantErr: make([]bool, len(kinds)),
	}

	var servers []*httptest.Server
	stop := make(chan struct{})

	for i, kind := range kinds {
		idx := i
		switch kind {
		case kindOK:
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				th.hits[idx].Add(1)
				w.Header().Set("Content-Type", "application/graphql-response+json")
				_, _ = w.Write([]byte(`{"data":{"hero":{"name":"Luke"}}}`))
			}))
			servers = append(servers, srv)
			th.hosts[idx] = host.Host(srv.URL)

		case kindTimeout:
			srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				th.hits[idx].Add(1)
				select {
				case <-r.Context().Done():
				case <-stop:
				}
			}))
			servers = append(servers, srv)
			th.hosts[idx] = host.Host(srv.URL)
			th.wantErr[idx] = true

		case kindError:
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				th.hits[idx].Add(1)
				w.Header().Set("Content-Type", "application/graphql-response+json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"errors":[{"message":"upstream failure"}]}`))
			}))
			servers = append(servers, srv)
			th.hosts[idx] = host.Host(srv.URL)
			th.wantErr[idx] = true

		case kindUnreachable:
			// RFC 5737 TEST-NET-1: guaranteed unreachable.
			th.hosts[idx] = host.Host("http://192.0.2.1:1")
			th.wantErr[idx] = true
		}
	}

	th.cleanup = func() {
		close(stop)
		for _, srv := range servers {
			srv.Close()
		}
	}

	return th
}
