package httpclient

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func (c *Client) prepareBody(
	req *restfile.Request,
	resolver *vars.Resolver,
	opts Options,
) (io.Reader, error) {
	if req.Body.GraphQL != nil {
		return c.prepareGraphQLBody(req, resolver, opts)
	}

	fallbacks, allowRaw := resolveFileLookup(opts.BaseDir, opts)

	switch {
	case req.Body.FilePath != "":
		data, _, err := c.readFileWithFallback(
			req.Body.FilePath,
			opts.BaseDir,
			fallbacks,
			allowRaw,
			"body file",
		)
		if err != nil {
			return nil, err
		}

		if resolver != nil && req.Body.Options.ExpandTemplates {
			text := string(data)
			expanded, err := resolver.ExpandTemplates(text)
			if err != nil {
				return nil, errdef.Wrap(errdef.CodeHTTP, err, "expand body file templates")
			}

			processed, procErr := c.injectBodyIncludes(expanded, opts.BaseDir, fallbacks, allowRaw)
			if procErr != nil {
				return nil, procErr
			}
			return strings.NewReader(processed), nil
		}
		return bytes.NewReader(data), nil
	case req.Body.Text != "":
		expanded := req.Body.Text
		if resolver != nil {
			var err error
			expanded, err = resolver.ExpandTemplates(req.Body.Text)
			if err != nil {
				return nil, errdef.Wrap(errdef.CodeHTTP, err, "expand body template")
			}
		}
		processed, err := c.injectBodyIncludes(expanded, opts.BaseDir, fallbacks, allowRaw)
		if err != nil {
			return nil, err
		}
		return strings.NewReader(processed), nil
	default:
		return nil, nil
	}
}

// GET requests put everything in query params, POST uses JSON body.
// Variables need special handling since they must be valid JSON in both cases.
func (c *Client) prepareGraphQLBody(
	req *restfile.Request,
	resolver *vars.Resolver,
	opts Options,
) (io.Reader, error) {
	gql := req.Body.GraphQL
	fallbacks, allowRaw := resolveFileLookup(opts.BaseDir, opts)

	query, err := c.gqlQuery(gql, resolver, opts.BaseDir, fallbacks, allowRaw)
	if err != nil {
		return nil, err
	}

	op, err := gqlOpName(gql, resolver)
	if err != nil {
		return nil, err
	}

	varsMap, varsJSON, err := c.gqlVars(gql, resolver, opts.BaseDir, fallbacks, allowRaw)
	if err != nil {
		return nil, err
	}

	if strings.EqualFold(req.Method, "GET") {
		if err := setGraphQLQuery(req, resolver, query, op, varsJSON); err != nil {
			return nil, err
		}
		return nil, nil
	}

	return buildGraphQLPayload(query, op, varsMap)
}

func (c *Client) gqlQuery(
	gql *restfile.GraphQLBody,
	resolver *vars.Resolver,
	baseDir string,
	fallbacks []string,
	allowRaw bool,
) (string, error) {
	query, err := c.graphQLSectionContent(
		gql.Query,
		gql.QueryFile,
		baseDir,
		fallbacks,
		allowRaw,
		"GraphQL query",
	)
	if err != nil {
		return "", err
	}

	if resolver != nil {
		if expanded, expandErr := resolver.ExpandTemplates(query); expandErr == nil {
			query = expanded
		} else {
			return "", errdef.Wrap(errdef.CodeHTTP, expandErr, "expand graphql query")
		}
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return "", errdef.New(errdef.CodeHTTP, "graphql query is empty")
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
		return "", errdef.Wrap(errdef.CodeHTTP, err, "expand graphql operation name")
	}
	return strings.TrimSpace(expanded), nil
}

func (c *Client) gqlVars(
	gql *restfile.GraphQLBody,
	resolver *vars.Resolver,
	baseDir string,
	fallbacks []string,
	allowRaw bool,
) (map[string]interface{}, string, error) {
	raw, err := c.graphQLSectionContent(
		gql.Variables,
		gql.VariablesFile,
		baseDir,
		fallbacks,
		allowRaw,
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
			return nil, "", errdef.Wrap(errdef.CodeHTTP, expandErr, "expand graphql variables")
		}
	}

	parsed, parseErr := decodeGraphQLVariables(raw)
	if parseErr != nil {
		return nil, "", parseErr
	}

	normalised, marshalErr := json.Marshal(parsed)
	if marshalErr != nil {
		return nil, "", errdef.Wrap(errdef.CodeHTTP, marshalErr, "encode graphql variables")
	}
	return parsed, string(normalised), nil
}

func setGraphQLQuery(
	req *restfile.Request,
	resolver *vars.Resolver,
	query, op, varsJSON string,
) error {
	expandedURL := strings.TrimSpace(req.URL)
	if resolver != nil {
		if expanded, expandErr := resolver.ExpandTemplates(expandedURL); expandErr == nil {
			expandedURL = strings.TrimSpace(expanded)
		} else {
			return errdef.Wrap(errdef.CodeHTTP, expandErr, "expand graphql request url")
		}
	}
	if expandedURL == "" {
		return errdef.New(errdef.CodeHTTP, "graphql request url is empty")
	}

	parsedURL, urlErr := url.Parse(expandedURL)
	if urlErr != nil {
		return errdef.Wrap(errdef.CodeHTTP, urlErr, "parse graphql request url")
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
	req.URL = parsedURL.String()
	return nil
}

func buildGraphQLPayload(
	query, op string,
	vars map[string]interface{},
) (io.Reader, error) {
	payload := map[string]interface{}{
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
		return nil, errdef.Wrap(errdef.CodeHTTP, err, "encode graphql payload")
	}
	return bytes.NewReader(body), nil
}

func (c *Client) graphQLSectionContent(
	inline, filePath, baseDir string,
	fallbacks []string,
	allowRaw bool,
	label string,
) (string, error) {
	inline = strings.TrimSpace(inline)
	if inline != "" {
		return inline, nil
	}

	if filePath == "" {
		return "", nil
	}

	data, _, err := c.readFileWithFallback(
		filePath,
		baseDir,
		fallbacks,
		allowRaw,
		strings.ToLower(label),
	)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Second Decode call checks for trailing garbage after the JSON object.
// Without this, extra content would silently get ignored.
func decodeGraphQLVariables(raw string) (map[string]interface{}, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var payload map[string]interface{}
	if err := decoder.Decode(&payload); err != nil {
		return nil, errdef.Wrap(errdef.CodeHTTP, err, "parse graphql variables")
	}

	if err := decoder.Decode(new(interface{})); err != io.EOF {
		if err == nil {
			return nil, errdef.New(errdef.CodeHTTP, "unexpected trailing data in graphql variables")
		}
		return nil, errdef.Wrap(errdef.CodeHTTP, err, "parse graphql variables")
	}
	return payload, nil
}
