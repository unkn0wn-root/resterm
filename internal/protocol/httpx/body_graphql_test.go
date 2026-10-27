package httpx

import (
	"net/http"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestBuildHTTPRequestGraphQLGolden(t *testing.T) {
	files := mapFS{
		"q.graphql": []byte("query Ping($id: ID!) { ping(id: $id) }\n"),
		"v.json":    []byte(`{"id": "{{id}}"}`),
	}
	env := vars.NewMapProvider("env", map[string]string{"id": "42", "op": "Ping", "base": "https://example.com"})
	c := NewClient(files)
	const (
		endpoint = "https://example.com/graphql"
		jsonCT   = "application/json"
	)
	tests := []struct {
		name   string
		method string
		url    string
		ct     string
		gql    restfile.GraphQLBody
		target string
		reqURL string
		body   string
		wantCT string
	}{
		{
			name:   "post inline",
			method: http.MethodPost,
			url:    endpoint,
			gql:    restfile.GraphQLBody{Query: "  query { ping }  "},
			target: endpoint,
			reqURL: endpoint,
			body:   `{"query":"query { ping }"}`,
			wantCT: jsonCT,
		},
		{
			name:   "post operation and variables",
			method: http.MethodPost,
			url:    endpoint,
			gql: restfile.GraphQLBody{
				Query:         "query Ping($id: ID!, $n: Float) { ping(id: $id, n: $n) }",
				OperationName: " Ping ",
				Variables:     `{ "n": 1.50, "id": "{{id}}", "tags": ["<a>", "&"], "deep": {"z": null, "a": true} }`,
			},
			target: endpoint,
			reqURL: endpoint,
			body: `{"operationName":"Ping","query":"query Ping($id: ID!, $n: Float) { ping(id: $id, n: $n) }",` +
				`"variables":{"deep":{"a":true,"z":null},"id":"42","n":1.50,"tags":["\u003ca\u003e","\u0026"]}}`,
			wantCT: jsonCT,
		},
		{
			name:   "post empty variables",
			method: http.MethodPost,
			url:    endpoint,
			gql:    restfile.GraphQLBody{Query: "query { ping }", Variables: "{}"},
			target: endpoint,
			reqURL: endpoint,
			body:   `{"query":"query { ping }","variables":{}}`,
			wantCT: jsonCT,
		},
		{
			name:   "post null variables",
			method: http.MethodPost,
			url:    endpoint,
			gql:    restfile.GraphQLBody{Query: "query { ping }", Variables: "null"},
			target: endpoint,
			reqURL: endpoint,
			body:   `{"query":"query { ping }"}`,
			wantCT: jsonCT,
		},
		{
			name:   "post files",
			method: http.MethodPost,
			url:    endpoint,
			gql:    restfile.GraphQLBody{QueryFile: "q.graphql", VariablesFile: "v.json"},
			target: endpoint,
			reqURL: endpoint,
			body:   `{"query":"query Ping($id: ID!) { ping(id: $id) }","variables":{"id":"42"}}`,
			wantCT: jsonCT,
		},
		{
			name:   "post templates",
			method: http.MethodPost,
			url:    "{{base}}/graphql",
			gql:    restfile.GraphQLBody{Query: `query { user(id: "{{id}}") { name } }`, OperationName: "{{op}}"},
			target: endpoint,
			reqURL: "{{base}}/graphql",
			body:   `{"operationName":"Ping","query":"query { user(id: \"42\") { name } }"}`,
			wantCT: jsonCT,
		},
		{
			name:   "post keeps its content type",
			method: http.MethodPost,
			url:    endpoint,
			ct:     "application/json; charset=utf-8",
			gql:    restfile.GraphQLBody{Query: "query { ping }"},
			target: endpoint,
			reqURL: endpoint,
			body:   `{"query":"query { ping }"}`,
			wantCT: "application/json; charset=utf-8",
		},
		{
			name:   "get drops stale parameters",
			method: http.MethodGet,
			url:    endpoint + "?existing=1&operationName=Old&variables=old",
			gql:    restfile.GraphQLBody{Query: "query { ping }"},
			target: endpoint + "?existing=1&query=query+%7B+ping+%7D",
			reqURL: endpoint + "?existing=1&query=query+%7B+ping+%7D",
		},
		{
			name:   "get operation and variables",
			method: http.MethodGet,
			url:    endpoint + "?existing=1",
			gql: restfile.GraphQLBody{
				Query:         "query Ping($id: ID!) { ping(id: $id) }",
				OperationName: "Ping",
				Variables:     `{"id": "{{id}}", "n": 1.50}`,
			},
			target: endpoint + "?existing=1&operationName=Ping&query=query+Ping%28%24id%3A+ID%21%29+%7B+ping%28id%3A+%24id%29+%7D" +
				"&variables=%7B%22id%22%3A%2242%22%2C%22n%22%3A1.50%7D",
			reqURL: endpoint + "?existing=1&operationName=Ping&query=query+Ping%28%24id%3A+ID%21%29+%7B+ping%28id%3A+%24id%29+%7D" +
				"&variables=%7B%22id%22%3A%2242%22%2C%22n%22%3A1.50%7D",
		},
		{
			name:   "get templated url and empty variables",
			method: http.MethodGet,
			url:    "{{base}}/graphql",
			gql:    restfile.GraphQLBody{Query: "query { ping }", Variables: "{}"},
			target: endpoint + "?query=query+%7B+ping+%7D&variables=%7B%7D",
			reqURL: endpoint + "?query=query+%7B+ping+%7D&variables=%7B%7D",
		},
		{
			name:   "get null variables",
			method: http.MethodGet,
			url:    endpoint,
			gql:    restfile.GraphQLBody{Query: "query { ping }", Variables: "null"},
			target: endpoint + "?query=query+%7B+ping+%7D&variables=null",
			reqURL: endpoint + "?query=query+%7B+ping+%7D&variables=null",
		},
		{
			name:   "get files",
			method: "get",
			url:    endpoint,
			gql:    restfile.GraphQLBody{QueryFile: "q.graphql", VariablesFile: "v.json", OperationName: "{{op}}"},
			target: endpoint + "?operationName=Ping&query=query+Ping%28%24id%3A+ID%21%29+%7B+ping%28id%3A+%24id%29+%7D" +
				"&variables=%7B%22id%22%3A%2242%22%7D",
			reqURL: endpoint + "?operationName=Ping&query=query+Ping%28%24id%3A+ID%21%29+%7B+ping%28id%3A+%24id%29+%7D" +
				"&variables=%7B%22id%22%3A%2242%22%7D",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gql := tt.gql
			req := &restfile.Request{Method: tt.method, URL: tt.url, Body: restfile.BodySource{GraphQL: &gql}}
			if tt.ct != "" {
				req.Headers = http.Header{"Content-Type": {tt.ct}}
			}
			httpReq, _, body, err := c.BuildHTTPRequest(t.Context(), req, vars.NewResolver(env), Options{})
			if err != nil {
				t.Fatalf("BuildHTTPRequest: %v", err)
			}
			if got := httpReq.URL.String(); got != tt.target {
				t.Fatalf("target = %q, want %q", got, tt.target)
			}
			if req.URL != tt.reqURL {
				t.Fatalf("req.URL = %q, want %q", req.URL, tt.reqURL)
			}
			if string(body) != tt.body {
				t.Fatalf("body = %s, want %s", body, tt.body)
			}
			if got := httpReq.Header.Get("Content-Type"); got != tt.wantCT {
				t.Fatalf("Content-Type = %q, want %q", got, tt.wantCT)
			}
		})
	}
}

func TestBuildHTTPRequestGraphQLErrors(t *testing.T) {
	const ping = "query { ping }"
	c := NewClient(mapFS{})
	tests := []struct {
		name   string
		method string
		gql    restfile.GraphQLBody
		want   string
	}{
		{
			name:   "empty query",
			method: http.MethodPost,
			gql:    restfile.GraphQLBody{Query: "  "},
			want:   "graphql query is empty",
		},
		{
			name:   "missing query file",
			method: http.MethodPost,
			gql:    restfile.GraphQLBody{QueryFile: "nope.graphql"},
			want:   "read graphql query nope.graphql (last tried nope.graphql): file does not exist",
		},
		{
			name:   "unknown query variable",
			method: http.MethodPost,
			gql:    restfile.GraphQLBody{Query: "query { {{nope}} }"},
			want:   "expand graphql query: undefined variable: nope",
		},
		{
			name:   "unknown operation variable",
			method: http.MethodGet,
			gql:    restfile.GraphQLBody{Query: ping, OperationName: "{{nope}}"},
			want:   "expand graphql operation name: undefined variable: nope",
		},
		{
			name:   "unknown variables variable",
			method: http.MethodGet,
			gql:    restfile.GraphQLBody{Query: ping, Variables: `{"a": "{{nope}}"}`},
			want:   "expand graphql variables: undefined variable: nope",
		},
		{
			name:   "variables not an object",
			method: http.MethodPost,
			gql:    restfile.GraphQLBody{Query: ping, Variables: `[1]`},
			want:   "parse graphql variables: json: cannot unmarshal array into Go value of type map[string]interface {}",
		},
		{
			name:   "variables with trailing data",
			method: http.MethodGet,
			gql:    restfile.GraphQLBody{Query: ping, Variables: `{} {}`},
			want:   "unexpected trailing data in graphql variables",
		},
		{
			name:   "missing variables file",
			method: http.MethodGet,
			gql:    restfile.GraphQLBody{Query: ping, VariablesFile: "nope.json"},
			want:   "read graphql variables nope.json (last tried nope.json): file does not exist",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gql := tt.gql
			req := &restfile.Request{
				Method: tt.method,
				URL:    "https://example.com/graphql",
				Body:   restfile.BodySource{GraphQL: &gql},
			}
			_, _, _, err := c.BuildHTTPRequest(t.Context(), req, vars.NewResolver(), Options{})
			if err == nil || err.Error() != tt.want {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}
