package graphql_test

import (
	"context"
	"testing"

	"github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/gqltesting"
)

type emptySelectionUser struct{}

func (emptySelectionUser) Name() string { return "Alice" }

type emptySelectionResolver struct{}

func (emptySelectionResolver) Hello() string { return "Hello world!" }

func (emptySelectionResolver) User() emptySelectionUser { return emptySelectionUser{} }

// An empty selection set on a field ("field {}") is treated as no selection set (kumparan fork),
// so it is accepted on scalar fields, as in graphql-go before v1.10.
func TestEmptyFieldSelectionSet(t *testing.T) {
	t.Parallel()

	schema := graphql.MustParseSchema(`
		schema {
			query: Query
		}

		type Query {
			hello: String!
			user: User!
		}

		type User {
			name: String!
		}
	`, &emptySelectionResolver{})

	gqltesting.RunTests(t, []*gqltesting.Test{
		{
			Schema:         schema,
			Query:          `{ hello {} }`,
			ExpectedResult: `{"hello": "Hello world!"}`,
		},
		{
			Schema:         schema,
			Query:          `{ user { name {} } }`,
			ExpectedResult: `{"user": {"name": "Alice"}}`,
		},
	})

	for _, q := range []string{`{ user {} }`, `{}`, `query {}`} {
		resp := schema.Exec(context.Background(), q, "", nil)
		if len(resp.Errors) == 0 {
			t.Errorf("%s: expected an error, got data %s", q, resp.Data)
		}
	}
}
