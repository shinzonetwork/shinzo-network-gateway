package endpoint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/shinzonetwork/shinzo-network-gateway/host"
)

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
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
