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
