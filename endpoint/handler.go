package endpoint

import (
	"io"
	"net/http"
	"sync"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"

	"github.com/shinzonetwork/shinzo-network-gateway/host"
)

type HostsSelector interface {
	SelectHosts(collections []string) ([]host.Host, error)
}

type Handler struct {
	extractor CollectionsExtractor
	selector  HostsSelector
}

type hostResponse struct {
	response string
	err      error
}

var _ http.Handler = &Handler{}

func NewHandler(extractor CollectionsExtractor, selector HostsSelector) *Handler {
	return &Handler{
		extractor: extractor,
		selector:  selector,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		// TODO(tzdybal): handle errors
	}

	collections, err := h.extractor.ExtractCollections(string(body))
	if err != nil {
		// TODO(tzdybal): handle errors
	}

	hosts, err := h.selector.SelectHosts(collections)
	if err != nil {
		// TODO(tzdybal): handle errors
	}

	responses := h.getHostsReponses(hosts, body)

	h.composeResponse(w, responses)
}

func (h *Handler) getHostsReponses(hosts []host.Host, body []byte) []hostResponse {
	responses := make([]hostResponse, len(hosts))
	wg := &sync.WaitGroup{}
	for i, host := range hosts {
		wg.Go(func() {
			responses[i] = h.queryHost(host, string(body))
		})
	}
	wg.Wait()

	return responses
}

func (h *Handler) queryHost(host host.Host, body string) hostResponse {
	panic("implement me!")
}

func (h *Handler) composeResponse(w http.ResponseWriter, responses []hostResponse) {
	panic("implement me!")
}

// CollectionsExtractor defines interface for extracting root collections from GraphQL queries.
type CollectionsExtractor interface {
	ExtractCollections(graphql string) ([]string, error)
}

// DefaultCollectionExtractor provides default implementation for root collections extraction.
type DefaultCollectionExtractor struct{}

// ExtractCollections parses GraphQL into AST and then traverse to get the root collections.
func (e *DefaultCollectionExtractor) ExtractCollections(graphql string) ([]string, error) {
	query, err := parser.ParseQuery(&ast.Source{Input: graphql})
	if err != nil {
		return nil, err
	}

	rootCollections := make([]string, 1)
	for _, op := range query.Operations {
		for _, sel := range op.SelectionSet {
			if field, ok := sel.(*ast.Field); ok {
				rootCollections = append(rootCollections, field.Name)
			}
		}
	}
	return rootCollections, nil
}
