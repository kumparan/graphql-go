// Package directives provides hooks for custom field directives declared in the schema.
package directives

import (
	"context"

	"github.com/graph-gophers/graphql-go/ast"
)

// Visitor handles a custom field directive declared in the schema.
// Before is called before the field resolver runs and can abort it by returning an error.
// After is called with the resolver output and may replace it.
type Visitor interface {
	Before(ctx context.Context, directive *ast.Directive, input any) error
	After(ctx context.Context, directive *ast.Directive, output any) (any, error)
}
