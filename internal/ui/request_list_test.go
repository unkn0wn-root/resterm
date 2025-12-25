package ui

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestRequestListItemDescriptionFallbacks(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		item     requestListItem
		expected string
	}{
		{
			name: "rest with description",
			item: requestListItem{
				request: &restfile.Request{
					Method: "post",
					URL:    "/graphql",
					Metadata: restfile.RequestMetadata{
						Description: " Create widget ",
					},
				},
				line: 12,
			},
			expected: "Create widget\nPOST /graphql",
		},
		{
			name: "rest absolute with description",
			item: requestListItem{
				request: &restfile.Request{
					Method: "post",
					URL:    "https://example.com/graphql",
					Metadata: restfile.RequestMetadata{
						Description: " Create absolute widget ",
					},
				},
				line: 8,
			},
			expected: "Create absolute widget\nPOST https://example.com/graphql",
		},
		{
			name: "rest without description",
			item: requestListItem{
				request: &restfile.Request{Method: "get", URL: "http://example.com"},
				line:    42,
			},
			expected: "GET\nhttp://example.com",
		},
		{
			name: "rest absolute path without description",
			item: requestListItem{
				request: &restfile.Request{Method: "get", URL: "https://example.com/api/v1?q=foo"},
				line:    3,
			},
			expected: "GET /api/v1?q=foo\nhttps://example.com",
		},
		{
			name: "rest templated path without description",
			item: requestListItem{
				request: &restfile.Request{
					Method: "get",
					URL:    "http://localhost:8080/items/{{vars.workflow.itemId}}",
				},
				line: 4,
			},
			expected: "GET /items/{{vars.workflow.itemId}}\nhttp://localhost:8080",
		},
		{
			name: "rest fallback to line",
			item: requestListItem{
				request: &restfile.Request{Method: "delete"},
				line:    7,
			},
			expected: "DELETE\nLine 7",
		},
		{
			name: "grpc full method",
			item: requestListItem{
				request: &restfile.Request{
					Method: "grpc",
					GRPC: &restfile.GRPCRequest{
						FullMethod: "/helloworld.Greeter/SayHello",
					},
				},
				line: 5,
			},
			expected: "GRPC\n/helloworld.Greeter/SayHello",
		},
		{
			name: "grpc with description",
			item: requestListItem{
				request: &restfile.Request{
					Method:   "grpc",
					Metadata: restfile.RequestMetadata{Description: " Call greeting "},
					GRPC: &restfile.GRPCRequest{
						FullMethod: "/helloworld.Greeter/SayHello",
					},
				},
				line: 6,
			},
			expected: "Call greeting\nGRPC /helloworld.Greeter/SayHello",
		},
		{
			name: "grpc composed service method",
			item: requestListItem{
				request: &restfile.Request{
					Method: "grpc",
					GRPC: &restfile.GRPCRequest{
						Service: "helloworld.Greeter",
						Method:  "SayHello",
					},
				},
				line: 18,
			},
			expected: "GRPC\nhelloworld.Greeter.SayHello",
		},
		{
			name: "grpc no identifiers fallback to line",
			item: requestListItem{
				request: &restfile.Request{
					Method: "grpc",
					GRPC:   &restfile.GRPCRequest{},
				},
				line: 9,
			},
			expected: "GRPC\nLine 9",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := tc.item.Description()
			if got != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestRequestListItemDescriptionExpandsConstants(t *testing.T) {
	t.Parallel()

	doc := &restfile.Document{
		Constants: []restfile.Constant{{Name: "svc.http", Value: "http://localhost:8080"}},
		Requests: []*restfile.Request{
			{
				Method: "get",
				URL:    "{{svc.http}}/api/items",
				LineRange: restfile.LineRange{
					Start: 5,
				},
			},
		},
	}

	var model Model
	items, _ := model.buildRequestItems(doc)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	desc := items[0].Description()
	expected := "GET /api/items\nhttp://localhost:8080"
	if desc != expected {
		t.Fatalf("expected %q, got %q", expected, desc)
	}
}

func TestRequestListFilterValueExpandsSafeValues(t *testing.T) {
	t.Parallel()

	doc := &restfile.Document{
		Constants: []restfile.Constant{{Name: "svc.http", Value: "http://localhost:8080"}},
		Requests: []*restfile.Request{
			{
				Method: "get",
				URL:    "{{svc.http}}/users",
				Metadata: restfile.RequestMetadata{
					Name:        "{{svc.http}} Users",
					Description: "List users",
				},
			},
		},
	}

	var model Model
	items, listItems := model.buildRequestItems(doc)
	if len(items) != 1 || len(listItems) != 1 {
		t.Fatalf("expected 1 item, got %d items and %d list items", len(items), len(listItems))
	}
	filter := items[0].FilterValue()
	if filter == "" || !strings.Contains(filter, "http://localhost:8080") {
		t.Fatalf("expected filter value to include expanded constant, got %q", filter)
	}
}

func TestRequestListFilterValueDoesNotLeakSecrets(t *testing.T) {
	t.Parallel()

	doc := &restfile.Document{
		Requests: []*restfile.Request{
			{
				Method: "get",
				URL:    "https://api/{{token}}/users",
				Variables: []restfile.Variable{{
					Name:   "token",
					Value:  "supersecret",
					Secret: true,
				}},
			},
		},
	}

	var model Model
	items, _ := model.buildRequestItems(doc)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	filter := items[0].FilterValue()
	if strings.Contains(filter, "supersecret") {
		t.Fatalf("filter value leaked secret: %q", filter)
	}
	if !strings.Contains(filter, "{{token}}") {
		t.Fatalf("expected unresolved secret placeholder to remain, got %q", filter)
	}
}
