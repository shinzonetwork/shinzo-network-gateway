package endpoint

import (
	"fmt"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

func parseQuery(graphql string) (*ast.QueryDocument, error) {
	if len(graphql) == 0 {
		return nil, ErrEmptyQuery
	}
	query, err := parser.ParseQuery(&ast.Source{Input: graphql})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrParse, err)
	}
	return query, nil
}

// rootFields returns the root-level field selections for every operation in query.
// It returns ErrUnsupportedSelection if a root selection is not a plain field
// (a fragment spread or inline fragment).
func rootFields(query *ast.QueryDocument) ([]*ast.Field, error) {
	var fields []*ast.Field
	for _, op := range query.Operations {
		for _, sel := range op.SelectionSet {
			field, ok := sel.(*ast.Field)
			if !ok {
				return nil, fmt.Errorf("%w: %T", ErrUnsupportedSelection, sel)
			}
			fields = append(fields, field)
		}
	}
	return fields, nil
}
