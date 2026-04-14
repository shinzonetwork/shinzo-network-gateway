package endpoint

import (
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

// CollectionsExtractor defines interface for extracting root collections from GraphQL queries.
type CollectionsExtractor interface {
	ExtractCollections(graphql string) ([]string, error)
}

// DefaultCollectionExtractor provides default implementation for root collections extraction.
type DefaultCollectionExtractor struct{}

// ExtractCollections parses GraphQL into AST and then traverse to get the root collections.
func (e *DefaultCollectionExtractor) ExtractCollections(graphql string) ([]string, error) {
	if len(graphql) == 0 {
		return nil, ErrEmptyQuery
	}
	query, err := parser.ParseQuery(&ast.Source{Input: graphql})
	if err != nil {
		return nil, err
	}

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
