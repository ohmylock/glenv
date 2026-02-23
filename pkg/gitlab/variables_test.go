//nolint:errcheck // test file
package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListVariables_SinglePage(t *testing.T) {
	vars := []Variable{
		{Key: "FOO", Value: "bar", VariableType: "env_var", EnvironmentScope: "*"},
		{Key: "SECRET", Value: "s3cr3t", VariableType: "env_var", Masked: true, EnvironmentScope: "*"},
		{Key: "CERT", Value: "-----BEGIN", VariableType: "file", EnvironmentScope: "production"},
	}

	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v4/projects/42/variables", r.URL.Path)
		assert.Equal(t, "100", r.URL.Query().Get("per_page"))
		// No X-Next-Page header → single page
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(vars)
	})

	result, err := client.ListVariables(context.Background(), "42", ListOptions{})
	require.NoError(t, err)
	require.Len(t, result, 3)
	assert.Equal(t, "FOO", result[0].Key)
	assert.Equal(t, "SECRET", result[1].Key)
	assert.Equal(t, "CERT", result[2].Key)
}

func TestListVariables_MultiPage(t *testing.T) {
	page1 := []Variable{
		{Key: "VAR1", Value: "v1", VariableType: "env_var", EnvironmentScope: "*"},
		{Key: "VAR2", Value: "v2", VariableType: "env_var", EnvironmentScope: "*"},
	}
	page2 := []Variable{
		{Key: "VAR3", Value: "v3", VariableType: "env_var", EnvironmentScope: "*"},
	}

	var callCount atomic.Int32
	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		count := callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if count == 1 {
			w.Header().Set("X-Next-Page", "2")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(page1)
		} else {
			// No X-Next-Page → last page
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(page2)
		}
	})

	result, err := client.ListVariables(context.Background(), "42", ListOptions{})
	require.NoError(t, err)
	require.Len(t, result, 3)
	assert.Equal(t, "VAR1", result[0].Key)
	assert.Equal(t, "VAR2", result[1].Key)
	assert.Equal(t, "VAR3", result[2].Key)
	assert.Equal(t, int32(2), callCount.Load())
}

func TestListVariables_Empty(t *testing.T) {
	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]Variable{})
	})

	result, err := client.ListVariables(context.Background(), "42", ListOptions{})
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestListVariables_EnvScope(t *testing.T) {
	var receivedScope string

	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		receivedScope = r.URL.Query().Get("filter[environment_scope]")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]Variable{})
	})

	_, err := client.ListVariables(context.Background(), "42", ListOptions{EnvironmentScope: "production"})
	require.NoError(t, err)
	assert.Equal(t, "production", receivedScope)
}

func TestCreateVariable(t *testing.T) {
	req := CreateRequest{
		Key:              "NEW_VAR",
		Value:            "new_value",
		VariableType:     "env_var",
		EnvironmentScope: "*",
		Protected:        false,
		Masked:           true,
	}

	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v4/projects/99/variables", r.URL.Path)

		var body CreateRequest
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)
		assert.Equal(t, req.Key, body.Key)
		assert.Equal(t, req.Value, body.Value)
		assert.Equal(t, req.VariableType, body.VariableType)
		assert.Equal(t, req.Masked, body.Masked)

		created := Variable{
			Key:              req.Key,
			Value:            req.Value,
			VariableType:     req.VariableType,
			EnvironmentScope: req.EnvironmentScope,
			Masked:           req.Masked,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(created)
	})

	result, err := client.CreateVariable(context.Background(), "99", req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "NEW_VAR", result.Key)
	assert.Equal(t, "new_value", result.Value)
	assert.True(t, result.Masked)
}

func TestUpdateVariable(t *testing.T) {
	req := CreateRequest{
		Key:              "EXISTING",
		Value:            "updated_value",
		VariableType:     "env_var",
		EnvironmentScope: "staging",
	}

	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/api/v4/projects/10/variables/EXISTING", r.URL.Path)
		assert.Equal(t, "staging", r.URL.Query().Get("filter[environment_scope]"))

		updated := Variable{
			Key:              req.Key,
			Value:            req.Value,
			VariableType:     req.VariableType,
			EnvironmentScope: req.EnvironmentScope,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(updated)
	})

	result, err := client.UpdateVariable(context.Background(), "10", req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "EXISTING", result.Key)
	assert.Equal(t, "updated_value", result.Value)
}

func TestDeleteVariable(t *testing.T) {
	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/v4/projects/7/variables/MY_VAR", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	})

	err := client.DeleteVariable(context.Background(), "7", "MY_VAR", "")
	require.NoError(t, err)
}

func TestDeleteVariable_WithScope(t *testing.T) {
	var receivedScope string

	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		receivedScope = r.URL.Query().Get("filter[environment_scope]")
		w.WriteHeader(http.StatusNoContent)
	})

	err := client.DeleteVariable(context.Background(), "7", "MY_VAR", "production")
	require.NoError(t, err)
	assert.Equal(t, "production", receivedScope)
}

func TestCreateVariable_Error(t *testing.T) {
	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, `{"message":"409 Variable already exists"}`)
	})

	_, err := client.CreateVariable(context.Background(), "99", CreateRequest{Key: "DUP"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "409")
}

func TestUpdateVariable_Error(t *testing.T) {
	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"404 Variable Not Found"}`)
	})

	_, err := client.UpdateVariable(context.Background(), "10", CreateRequest{Key: "MISSING"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestDeleteVariable_UnexpectedStatus(t *testing.T) {
	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"message":"500 Internal Server Error"}`)
	})

	err := client.DeleteVariable(context.Background(), "7", "MY_VAR", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestFilterByScope_ExactMatch(t *testing.T) {
	vars := []Variable{
		{Key: "A", EnvironmentScope: "production"},
		{Key: "B", EnvironmentScope: "staging"},
		{Key: "C", EnvironmentScope: "*"},
	}
	got := FilterByScope(vars, "production")
	require.Len(t, got, 2)
	assert.Equal(t, "A", got[0].Key)
	assert.Equal(t, "C", got[1].Key)
}

func TestFilterByScope_WildcardTarget(t *testing.T) {
	vars := []Variable{
		{Key: "A", EnvironmentScope: "production"},
		{Key: "B", EnvironmentScope: "*"},
		{Key: "C", EnvironmentScope: "staging"},
	}
	got := FilterByScope(vars, "*")
	require.Len(t, got, 1)
	assert.Equal(t, "B", got[0].Key)
}

func TestFilterByScope_EmptyScope(t *testing.T) {
	vars := []Variable{
		{Key: "A", EnvironmentScope: "production"},
		{Key: "B", EnvironmentScope: "*"},
	}
	got := FilterByScope(vars, "")
	require.Len(t, got, 2)
}

func TestFilterByScope_NoMatches(t *testing.T) {
	vars := []Variable{
		{Key: "A", EnvironmentScope: "staging"},
		{Key: "B", EnvironmentScope: "development"},
	}
	got := FilterByScope(vars, "production")
	assert.Empty(t, got)
}

func TestFilterByScope_EmptyInput(t *testing.T) {
	got := FilterByScope(nil, "production")
	assert.Empty(t, got)
}

func TestListVariables_URLEncoding(t *testing.T) {
	// Verify filter[environment_scope] is correctly URL-encoded
	var rawQuery string

	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]Variable{})
	})

	_, err := client.ListVariables(context.Background(), "1", ListOptions{EnvironmentScope: "staging"})
	require.NoError(t, err)

	parsed, err := url.ParseQuery(rawQuery)
	require.NoError(t, err)
	assert.Equal(t, "staging", parsed.Get("filter[environment_scope]"))
}

func TestListVariables_DecodeError(t *testing.T) {
	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `[bad json`)
	})

	_, err := client.ListVariables(context.Background(), "42", ListOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode")
}

func TestCreateVariable_DecodeError(t *testing.T) {
	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `not-json{`)
	})

	_, err := client.CreateVariable(context.Background(), "99", CreateRequest{Key: "X"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode")
}

func TestUpdateVariable_DecodeError(t *testing.T) {
	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `garbage`)
	})

	_, err := client.UpdateVariable(context.Background(), "10", CreateRequest{Key: "X"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode")
}

func TestListVariables_NextPage_Zero(t *testing.T) {
	var callCount int32

	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Next-Page", "0")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]Variable{{Key: "A", EnvironmentScope: "*"}})
	})

	vars, err := client.ListVariables(context.Background(), "42", ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, int32(1), callCount, "pagination should stop when X-Next-Page is 0")
	require.Len(t, vars, 1)
	assert.Equal(t, "A", vars[0].Key)
}
