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

// ExtractCollections walks the parsed query's root field selections to get the root collections.
func (e *DefaultCollectionExtractor) ExtractCollections(query *ast.QueryDocument) ([]string, error) {
	fields, err := rootFields(query)
	if err != nil {
		return nil, err
	}
	rootCollections := make([]string, 0, len(fields))
	for _, field := range fields {
		rootCollections = append(rootCollections, field.Name)
	}
	return rootCollections, nil
}
