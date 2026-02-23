package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// readErrorBody reads up to 512 bytes from the response body for error diagnostics.
// It drains any remaining bytes so the HTTP transport can reuse the connection.
func readErrorBody(resp *http.Response) string {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512))
	_, _ = io.Copy(io.Discard, resp.Body)
	if err != nil || len(body) == 0 {
		return ""
	}
	return ": " + string(body)
}

// Variable represents a GitLab CI/CD project variable.
type Variable struct {
	Key              string `json:"key"`
	Value            string `json:"value"`
	VariableType     string `json:"variable_type"`
	EnvironmentScope string `json:"environment_scope"`
	Protected        bool   `json:"protected"`
	Masked           bool   `json:"masked"`
	Raw              bool   `json:"raw"`
}

// FilterByScope filters variables by environment scope on the client side.
// GitLab API does not reliably filter by environment_scope on the LIST endpoint
// (see https://gitlab.com/gitlab-org/gitlab/-/issues/343169), so we do it ourselves.
//
// Filtering rules:
//   - empty scope: return all variables unfiltered
//   - scope == "*": return only variables with EnvironmentScope == "*"
//   - specific scope: return variables with EnvironmentScope == scope OR EnvironmentScope == "*"
func FilterByScope(vars []Variable, scope string) []Variable {
	if scope == "" {
		return vars
	}
	result := make([]Variable, 0, len(vars))
	for _, v := range vars {
		if v.EnvironmentScope == scope || (scope != "*" && v.EnvironmentScope == "*") {
			result = append(result, v)
		}
	}
	return result
}

// CreateRequest is the payload for creating or updating a variable.
type CreateRequest struct {
	Key              string `json:"key"`
	Value            string `json:"value"`
	VariableType     string `json:"variable_type"`
	EnvironmentScope string `json:"environment_scope"`
	Protected        bool   `json:"protected"`
	Masked           bool   `json:"masked"`
	Raw              bool   `json:"raw"`
}

// ListOptions controls pagination and filtering for ListVariables.
type ListOptions struct {
	EnvironmentScope string
	Page             int
	PerPage          int
}

// ListVariables returns all variables for the given project, following pagination.
func (c *Client) ListVariables(ctx context.Context, projectID string, opts ListOptions) ([]Variable, error) {
	perPage := opts.PerPage
	if perPage <= 0 {
		perPage = 100
	}

	var all []Variable
	page := 1
	if opts.Page > 0 {
		page = opts.Page
	}

	const maxPages = 1000
	for pageNum := 0; pageNum < maxPages; pageNum++ {
		q := url.Values{}
		q.Set("per_page", strconv.Itoa(perPage))
		q.Set("page", strconv.Itoa(page))
		if opts.EnvironmentScope != "" {
			q.Set("filter[environment_scope]", opts.EnvironmentScope)
		}

		apiURL := fmt.Sprintf("%s/api/v4/projects/%s/variables?%s", c.cfg.BaseURL, url.PathEscape(projectID), q.Encode())
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("gitlab: list variables: build request: %w", err)
		}

		resp, err := c.Do(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("gitlab: list variables: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			msg := readErrorBody(resp)
			_ = resp.Body.Close()
			return nil, fmt.Errorf("gitlab: list variables: unexpected status %d%s", resp.StatusCode, msg)
		}

		var pageVars []Variable
		decodeErr := json.NewDecoder(resp.Body).Decode(&pageVars)
		_ = resp.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("gitlab: list variables: decode: %w", decodeErr)
		}
		all = append(all, pageVars...)

		nextPage := resp.Header.Get("X-Next-Page")
		if nextPage == "" || nextPage == "0" {
			return all, nil
		}
		n, err := strconv.Atoi(nextPage)
		if err != nil || n <= page {
			return all, nil
		}
		page = n
	}

	return nil, fmt.Errorf("gitlab: list variables: exceeded %d pages; possible pagination loop", maxPages)
}

// CreateVariable creates a new CI/CD variable for the given project.
func (c *Client) CreateVariable(ctx context.Context, projectID string, r CreateRequest) (*Variable, error) {
	body, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("gitlab: create variable: encode: %w", err)
	}

	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/variables", c.cfg.BaseURL, url.PathEscape(projectID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gitlab: create variable: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("gitlab: create variable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("gitlab: create variable: unexpected status %d%s", resp.StatusCode, readErrorBody(resp))
	}

	var v Variable
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, fmt.Errorf("gitlab: create variable: decode: %w", err)
	}
	return &v, nil
}

// UpdateVariable updates an existing CI/CD variable identified by r.Key and r.EnvironmentScope.
func (c *Client) UpdateVariable(ctx context.Context, projectID string, r CreateRequest) (*Variable, error) {
	body, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("gitlab: update variable: encode: %w", err)
	}

	q := url.Values{}
	if r.EnvironmentScope != "" {
		q.Set("filter[environment_scope]", r.EnvironmentScope)
	}

	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/variables/%s", c.cfg.BaseURL, url.PathEscape(projectID), url.PathEscape(r.Key))
	if len(q) > 0 {
		apiURL += "?" + q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gitlab: update variable: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("gitlab: update variable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab: update variable: unexpected status %d%s", resp.StatusCode, readErrorBody(resp))
	}

	var v Variable
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, fmt.Errorf("gitlab: update variable: decode: %w", err)
	}
	return &v, nil
}

// DeleteVariable removes a CI/CD variable from the given project.
// envScope is optional; pass "" to omit the filter.
func (c *Client) DeleteVariable(ctx context.Context, projectID, key, envScope string) error {
	q := url.Values{}
	if envScope != "" {
		q.Set("filter[environment_scope]", envScope)
	}

	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/variables/%s", c.cfg.BaseURL, url.PathEscape(projectID), url.PathEscape(key))
	if len(q) > 0 {
		apiURL += "?" + q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, apiURL, nil)
	if err != nil {
		return fmt.Errorf("gitlab: delete variable: build request: %w", err)
	}

	resp, err := c.Do(ctx, req)
	if err != nil {
		return fmt.Errorf("gitlab: delete variable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("gitlab: delete variable: unexpected status %d%s", resp.StatusCode, readErrorBody(resp))
	}
	return nil
}
