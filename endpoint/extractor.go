package endpoint

import (
	"github.com/vektah/gqlparser/v2/ast"
)

// CollectionsExtractor defines interface for extracting root collections from GraphQL queries.
type CollectionsExtractor interface {
	ExtractCollections(query *ast.QueryDocument) ([]string, error)
}

// DefaultCollectionExtractor provides default implementation for root collections extraction.
type DefaultCollectionExtractor struct{}

// ExtractCollections parses GraphQL into AST and then traverse to get the root collections.
func (e *DefaultCollectionExtractor) ExtractCollections(query *ast.QueryDocument) ([]string, error) {
	rootCollections := make([]string, 0, 1)
	for _, op := range query.Operations {
		for _, sel := range op.SelectionSet {
			if field, ok := sel.(*ast.Field); ok {
				rootCollections = append(rootCollections, field.Name)
			}
		}
	}
	return rootCollections, nil
}
