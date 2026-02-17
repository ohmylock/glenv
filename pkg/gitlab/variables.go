package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

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

// CreateRequest is the payload for creating or updating a variable.
type CreateRequest struct {
	Key              string `json:"key"`
	Value            string `json:"value"`
	VariableType     string `json:"variable_type"`
	EnvironmentScope string `json:"environment_scope"`
	Protected        bool   `json:"protected"`
	Masked           bool   `json:"masked"`
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

	for {
		q := url.Values{}
		q.Set("per_page", strconv.Itoa(perPage))
		q.Set("page", strconv.Itoa(page))
		if opts.EnvironmentScope != "" {
			q.Set("filter[environment_scope]", opts.EnvironmentScope)
		}

		apiURL := fmt.Sprintf("%s/api/v4/projects/%s/variables?%s", c.cfg.BaseURL, projectID, q.Encode())
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("gitlab: list variables: build request: %w", err)
		}

		resp, err := c.Do(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("gitlab: list variables: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("gitlab: list variables: unexpected status %d", resp.StatusCode)
		}

		var pageVars []Variable
		decodeErr := json.NewDecoder(resp.Body).Decode(&pageVars)
		resp.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("gitlab: list variables: decode: %w", decodeErr)
		}
		all = append(all, pageVars...)

		nextPage := resp.Header.Get("X-Next-Page")
		if nextPage == "" || nextPage == "0" {
			break
		}
		n, err := strconv.Atoi(nextPage)
		if err != nil || n <= page {
			break
		}
		page = n
	}

	return all, nil
}

// GetVariable fetches a single variable by key. Returns nil, nil if not found (404).
func (c *Client) GetVariable(ctx context.Context, projectID, key, envScope string) (*Variable, error) {
	q := url.Values{}
	if envScope != "" {
		q.Set("filter[environment_scope]", envScope)
	}

	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/variables/%s", c.cfg.BaseURL, projectID, url.PathEscape(key))
	if len(q) > 0 {
		apiURL += "?" + q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("gitlab: get variable: build request: %w", err)
	}

	resp, err := c.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("gitlab: get variable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab: get variable: unexpected status %d", resp.StatusCode)
	}

	var v Variable
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, fmt.Errorf("gitlab: get variable: decode: %w", err)
	}
	return &v, nil
}

// CreateVariable creates a new CI/CD variable for the given project.
func (c *Client) CreateVariable(ctx context.Context, projectID string, r CreateRequest) (*Variable, error) {
	body, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("gitlab: create variable: encode: %w", err)
	}

	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/variables", c.cfg.BaseURL, projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gitlab: create variable: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("gitlab: create variable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("gitlab: create variable: unexpected status %d", resp.StatusCode)
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

	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/variables/%s", c.cfg.BaseURL, projectID, url.PathEscape(r.Key))
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab: update variable: unexpected status %d", resp.StatusCode)
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

	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/variables/%s", c.cfg.BaseURL, projectID, url.PathEscape(key))
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("gitlab: delete variable: unexpected status %d", resp.StatusCode)
	}
	return nil
}
