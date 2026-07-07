package httpclient

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type bodyPlan struct {
	rd  io.Reader
	url string
}

func (p bodyPlan) effectiveURL(defaultURL string) string {
	if p.url != "" {
		return p.url
	}
	return defaultURL
}

func (c *Client) prepareBody(
	req *restfile.Request,
	resolver *vars.Resolver,
	opts Options,
) (bodyPlan, error) {
	if req.Body.GraphQL != nil {
		return c.prepareGraphQLBody(req, resolver, opts)
	}

	lookup := newFileLookup(opts.BaseDir, opts)

	switch {
	case req.Body.FilePath != "":
		data, _, err := lookup.read(c, req.Body.FilePath, "body file")
		if err != nil {
			return bodyPlan{}, err
		}

		if resolver != nil && req.Body.Options.ExpandTemplates {
			text := string(data)
			expanded, err := resolver.ExpandTemplates(text)
			if err != nil {
				return bodyPlan{}, diag.WrapAsf(diag.ClassProtocol, err,
					"expand body file templates",
				)
			}

			return c.textBodyPlan(expanded, lookup, req)
		}
		return bodyPlan{rd: bytes.NewReader(data)}, nil
	case req.Body.Text != "":
		expanded := req.Body.Text
		if resolver != nil {
			var err error
			expanded, err = resolver.ExpandTemplates(req.Body.Text)
			if err != nil {
				return bodyPlan{}, diag.WrapAs(diag.ClassProtocol, err, "expand body template")
			}
		}
		return c.textBodyPlan(expanded, lookup, req)
	default:
		return bodyPlan{}, nil
	}
}

func (c *Client) textBodyPlan(
	body string,
	lookup fileLookup,
	req *restfile.Request,
) (bodyPlan, error) {
	processed, err := c.injectBodyIncludes(body, lookup, isMultipartRequest(req))
	if err != nil {
		return bodyPlan{}, err
	}
	return bodyPlan{rd: bytes.NewReader(processed)}, nil
}

// Multipart bodies need CRLF framing on the wire. The Content-Type header is
// what is actually sent (scripts may rewrite it after parse); Body.MimeType is
// the parse-time fallback.
func isMultipartRequest(req *restfile.Request) bool {
	ct := req.Headers.Get("Content-Type")
	if ct == "" {
		ct = req.Body.MimeType
	}
	return restfile.IsMultipartMime(ct)
}

// GET requests put everything in query params, POST uses JSON body.
// Variables need special handling since they must be valid JSON in both cases.
func (c *Client) prepareGraphQLBody(
	req *restfile.Request,
	resolver *vars.Resolver,
	opts Options,
) (bodyPlan, error) {
	gql := req.Body.GraphQL
	lookup := newFileLookup(opts.BaseDir, opts)

	query, err := c.gqlQuery(gql, resolver, lookup)
	if err != nil {
		return bodyPlan{}, err
	}

	op, err := gqlOpName(gql, resolver)
	if err != nil {
		return bodyPlan{}, err
	}

	varsMap, varsJSON, err := c.gqlVars(gql, resolver, lookup)
	if err != nil {
		return bodyPlan{}, err
	}

	if strings.EqualFold(req.Method, "GET") {
		url, err := buildGraphQLURL(req.URL, resolver, query, op, varsJSON)
		if err != nil {
			return bodyPlan{}, err
		}
		// Preserve the long-standing package behavior for callers that inspect
		// the restfile request after preparation.
		req.URL = url
		return bodyPlan{url: url}, nil
	}

	reader, err := buildGraphQLPayload(query, op, varsMap)
	if err != nil {
		return bodyPlan{}, err
	}
	return bodyPlan{rd: reader}, nil
}

func (c *Client) gqlQuery(
	gql *restfile.GraphQLBody,
	resolver *vars.Resolver,
	lookup fileLookup,
) (string, error) {
	query, err := c.graphQLSectionContent(
		gql.Query,
		gql.QueryFile,
		lookup,
		"GraphQL query",
	)
	if err != nil {
		return "", err
	}

	if resolver != nil {
		if expanded, expandErr := resolver.ExpandTemplates(query); expandErr == nil {
			query = expanded
		} else {
			return "", diag.WrapAs(diag.ClassProtocol, expandErr, "expand graphql query")
		}
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return "", diag.New(diag.ClassProtocol, "graphql query is empty")
	}

	return query, nil
}

func gqlOpName(gql *restfile.GraphQLBody, resolver *vars.Resolver) (string, error) {
	op := strings.TrimSpace(gql.OperationName)
	if op == "" || resolver == nil {
		return op, nil
	}
	expanded, err := resolver.ExpandTemplates(op)
	if err != nil {
		return "", diag.WrapAs(diag.ClassProtocol, err, "expand graphql operation name")
	}
	return strings.TrimSpace(expanded), nil
}

func (c *Client) gqlVars(
	gql *restfile.GraphQLBody,
	resolver *vars.Resolver,
	lookup fileLookup,
) (map[string]any, string, error) {
	raw, err := c.graphQLSectionContent(
		gql.Variables,
		gql.VariablesFile,
		lookup,
		"GraphQL variables",
	)
	if err != nil {
		return nil, "", err
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, "", nil
	}

	if resolver != nil {
		if expanded, expandErr := resolver.ExpandTemplates(raw); expandErr == nil {
			raw = strings.TrimSpace(expanded)
		} else {
			return nil, "", diag.WrapAs(diag.ClassProtocol, expandErr, "expand graphql variables")
		}
	}

	parsed, parseErr := decodeGraphQLVariables(raw)
	if parseErr != nil {
		return nil, "", parseErr
	}

	normalised, marshalErr := json.Marshal(parsed)
	if marshalErr != nil {
		return nil, "", diag.WrapAs(diag.ClassProtocol, marshalErr, "encode graphql variables")
	}
	return parsed, string(normalised), nil
}

func buildGraphQLURL(
	rawURL string,
	resolver *vars.Resolver,
	query, op, varsJSON string,
) (string, error) {
	expandedURL := rawURL
	if resolver != nil {
		if expanded, expandErr := resolver.ExpandTemplates(expandedURL); expandErr == nil {
			expandedURL = expanded
		} else {
			return "", diag.WrapAs(diag.ClassProtocol, expandErr, "expand graphql request url")
		}
	}
	if expandedURL == "" {
		return "", diag.New(diag.ClassProtocol, "graphql request url is empty")
	}

	parsedURL, urlErr := url.Parse(expandedURL)
	if urlErr != nil {
		return "", diag.WrapAs(diag.ClassProtocol, urlErr, "parse graphql request url")
	}

	values := parsedURL.Query()
	values.Set("query", query)
	if op != "" {
		values.Set("operationName", op)
	} else {
		values.Del("operationName")
	}

	if varsJSON != "" {
		values.Set("variables", varsJSON)
	} else {
		values.Del("variables")
	}

	parsedURL.RawQuery = values.Encode()
	return parsedURL.String(), nil
}

func buildGraphQLPayload(
	query, op string,
	vars map[string]any,
) (io.Reader, error) {
	payload := map[string]any{
		"query": query,
	}

	if op != "" {
		payload["operationName"] = op
	}

	if vars != nil {
		payload["variables"] = vars
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, diag.WrapAs(diag.ClassProtocol, err, "encode graphql payload")
	}
	return bytes.NewReader(body), nil
}

func (c *Client) graphQLSectionContent(
	inline, filePath string,
	lookup fileLookup,
	label string,
) (string, error) {
	inline = strings.TrimSpace(inline)
	if inline != "" {
		return inline, nil
	}

	if filePath == "" {
		return "", nil
	}

	data, _, err := lookup.read(c, filePath, strings.ToLower(label))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Second Decode call checks for trailing garbage after the JSON object.
// Without this, extra content would silently get ignored.
func decodeGraphQLVariables(raw string) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		return nil, diag.WrapAs(diag.ClassProtocol, err, "parse graphql variables")
	}

	if err := decoder.Decode(new(any)); err != io.EOF {
		if err == nil {
			return nil, diag.Newf(
				diag.ClassProtocol,
				"unexpected trailing data in graphql variables",
			)
		}
		return nil, diag.WrapAs(diag.ClassProtocol, err, "parse graphql variables")
	}
	return payload, nil
}
