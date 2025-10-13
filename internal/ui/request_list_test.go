package ui

import (
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
				request: &restfile.Request{Method: "get", URL: "http://localhost:8080/items/{{vars.workflow.itemId}}"},
				line:    4,
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
