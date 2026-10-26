package graphql_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/ast"
	"github.com/graph-gophers/graphql-go/directives"
	"github.com/graph-gophers/graphql-go/gqltesting"
)

type customDirectiveVisitor struct {
	beforeWasCalled bool
}

func (v *customDirectiveVisitor) Before(ctx context.Context, directive *ast.Directive, input any) error {
	v.beforeWasCalled = true
	return nil
}

func (v *customDirectiveVisitor) After(ctx context.Context, directive *ast.Directive, output any) (any, error) {
	if !v.beforeWasCalled {
		return nil, errors.New("before directive visitor method wasn't called")
	}

	if value, ok := directive.Arguments.Get("customAttribute"); ok {
		return fmt.Sprintf("Directive '%s' (with arg '%s') modified result: %s", directive.Name.Name, value.String(), output.(string)), nil
	}
	return fmt.Sprintf("Directive '%s' modified result: %s", directive.Name.Name, output.(string)), nil
}

func TestCustomDirective(t *testing.T) {
	t.Parallel()

	gqltesting.RunTests(t, []*gqltesting.Test{
		{
			Schema: graphql.MustParseSchema(`
				directive @customDirective on FIELD_DEFINITION

				schema {
					query: Query
				}

				type Query {
					hello_html: String! @customDirective
				}
			`, &helloSnakeResolver1{},
				graphql.DirectiveVisitors(map[string]directives.Visitor{
					"customDirective": &customDirectiveVisitor{},
				})),
			Query: `
				{
					hello_html
				}
			`,
			ExpectedResult: `
				{
					"hello_html": "Directive 'customDirective' modified result: Hello snake!"
				}
			`,
		},
		{
			Schema: graphql.MustParseSchema(`
				directive @customDirective(
					customAttribute: String!
				) on FIELD_DEFINITION

				schema {
					query: Query
				}

				type Query {
					say_hello(full_name: String!): String! @customDirective(customAttribute: hi)
				}
			`, &helloSnakeResolver1{},
				graphql.DirectiveVisitors(map[string]directives.Visitor{
					"customDirective": &customDirectiveVisitor{},
				})),
			Query: `
				{
					say_hello(full_name: "Johnny")
				}
			`,
			ExpectedResult: `
				{
					"say_hello": "Directive 'customDirective' (with arg 'hi') modified result: Hello Johnny!"
				}
			`,
		},
	})
}

type extensionsError struct {
	code string
}

func (e extensionsError) Error() string { return e.code + ": denied" }

func (e extensionsError) Extensions() map[string]any {
	return map[string]any{"code": e.code}
}

type ctxKey struct{}

// guardVisitor rejects the field unless the context carries a user, and records the Before input.
type guardVisitor struct {
	input []any
}

func (v *guardVisitor) Before(ctx context.Context, directive *ast.Directive, input any) error {
	v.input = input.([]any)
	if ctx.Value(ctxKey{}) == nil {
		return extensionsError{code: "Unauthenticated"}
	}
	return nil
}

func (v *guardVisitor) After(ctx context.Context, directive *ast.Directive, output any) (any, error) {
	return output, nil
}

type guardedResolver struct {
	calls  int
	Public string
}

func (r *guardedResolver) Secret(ctx context.Context, args struct{ Name string }) (string, error) {
	r.calls++
	return "secret " + args.Name, nil
}

func TestCustomDirectiveBefore(t *testing.T) {
	t.Parallel()

	schemaString := `
		directive @mustLogin on FIELD_DEFINITION

		schema {
			query: Query
		}

		type Query {
			secret(name: String!): String! @mustLogin
			public: String! @mustLogin
		}
	`

	t.Run("rejected", func(t *testing.T) {
		t.Parallel()
		visitor := &guardVisitor{}
		res := &guardedResolver{}
		schema := graphql.MustParseSchema(schemaString, res, graphql.UseFieldResolvers(),
			graphql.DirectiveVisitors(map[string]directives.Visitor{"mustLogin": visitor}))

		resp := schema.Exec(context.Background(), `{ secret(name: "x") }`, "", nil)
		got, err := json.Marshal(resp)
		if err != nil {
			t.Fatal(err)
		}
		want := `{"errors":[{"message":"Unauthenticated: denied","path":["secret"],"extensions":{"code":"Unauthenticated"}}],"data":null}`
		if string(got) != want {
			t.Errorf("got %s, want %s", got, want)
		}
		if res.calls != 0 {
			t.Errorf("resolver was called %d times, want 0", res.calls)
		}
		var ee extensionsError
		if len(resp.Errors) != 1 || !errors.As(resp.Errors[0].ResolverError, &ee) {
			t.Errorf("ResolverError should hold the visitor error, got %v", resp.Errors)
		}
	})

	t.Run("allowed", func(t *testing.T) {
		t.Parallel()
		visitor := &guardVisitor{}
		res := &guardedResolver{}
		schema := graphql.MustParseSchema(schemaString, res, graphql.UseFieldResolvers(),
			graphql.DirectiveVisitors(map[string]directives.Visitor{"mustLogin": visitor}))

		ctx := context.WithValue(context.Background(), ctxKey{}, "user")
		resp := schema.Exec(ctx, `{ secret(name: "x") }`, "", nil)
		if len(resp.Errors) > 0 {
			t.Fatalf("unexpected errors: %v", resp.Errors)
		}
		if string(resp.Data) != `{"secret":"secret x"}` {
			t.Errorf("got %s", resp.Data)
		}
		if res.calls != 1 {
			t.Errorf("resolver was called %d times, want 1", res.calls)
		}
		if len(visitor.input) != 2 {
			t.Fatalf("Before input = %v, want context and arguments", visitor.input)
		}
		if _, ok := visitor.input[0].(context.Context); !ok {
			t.Errorf("Before input[0] = %T, want context.Context", visitor.input[0])
		}
		if args, ok := visitor.input[1].(struct{ Name string }); !ok || args.Name != "x" {
			t.Errorf("Before input[1] = %#v, want packed arguments", visitor.input[1])
		}
	})

	t.Run("field resolver is not visited", func(t *testing.T) {
		t.Parallel()
		res := &guardedResolver{Public: "public"}
		schema := graphql.MustParseSchema(schemaString, res, graphql.UseFieldResolvers(),
			graphql.DirectiveVisitors(map[string]directives.Visitor{"mustLogin": &guardVisitor{}}))

		resp := schema.Exec(context.Background(), `{ public }`, "", nil)
		if len(resp.Errors) > 0 {
			t.Fatalf("unexpected errors: %v", resp.Errors)
		}
		if string(resp.Data) != `{"public":"public"}` {
			t.Errorf("got %s", resp.Data)
		}
	})
}
