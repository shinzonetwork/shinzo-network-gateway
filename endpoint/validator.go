package endpoint

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/vektah/gqlparser/v2/ast"
)

// Validator validates a parsed GraphQL request before it is forwarded upstream.
type Validator interface {
	Validate(req *ValidationRequest) error
}

// ValidationRequest contains the parsed query and request headers to validate.
type ValidationRequest struct {
	Query  *ast.QueryDocument
	Header http.Header
}

// LimitValidator ensures every root field specifies a positive limit argument
// not exceeding the configured maximum.
type LimitValidator struct {
	limit int
}

var _ Validator = &LimitValidator{}

// NewLimitValidator creates configured instance of LimitValidator.
func NewLimitValidator(limit int) *LimitValidator {
	return &LimitValidator{
		limit: limit,
	}
}

// Validate checks that each root field carries a valid limit argument.
func (v *LimitValidator) Validate(req *ValidationRequest) error {
	for _, op := range req.Query.Operations {
		for _, sel := range op.SelectionSet {
			field, ok := sel.(*ast.Field)
			if !ok {
				return fmt.Errorf("%w: %T", ErrUnsupportedSelection, sel)
			}
			limits := 0
			var arg *ast.Argument
			for _, a := range field.Arguments {
				if a.Name == "limit" {
					limits++
					arg = a
				}
			}
			switch limits {
			case 0:
				return fmt.Errorf("%w: %s", ErrMissingLimit, field.Name)
			case 1:
				if err := v.checkLimit(field.Name, arg.Value); err != nil {
					return err
				}
			default: // limits > 1
				return fmt.Errorf("%w: %s: duplicate limit argument", ErrInvalidLimit, field.Name)
			}
		}
	}
	return nil
}

// checkLimit verifies that a limit argument has a positive integer literal within limit.
func (v *LimitValidator) checkLimit(field string, val *ast.Value) error {
	if val == nil || val.Kind != ast.IntValue {
		return fmt.Errorf("%w: %s: limit must be an integer literal", ErrInvalidLimit, field)
	}
	// GraphQL Int is a signed 32-bit value; parse with that bound so out-of-range
	// literals are rejected rather than silently forwarded as invalid Ints.
	n, err := strconv.ParseInt(val.Raw, 10, 32)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrInvalidLimit, field, err)
	}
	if n <= 0 {
		return fmt.Errorf("%w: %s: limit must be positive, got %d", ErrInvalidLimit, field, n)
	}
	if n > int64(v.limit) {
		return fmt.Errorf("%w: %s: limit %d exceeds maximum %d", ErrInvalidLimit, field, n, v.limit)
	}
	return nil
}
