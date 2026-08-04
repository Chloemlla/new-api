package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/docs/openapi"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type openapiDocument struct {
	OpenAPI string                    `json:"openapi"`
	Info    map[string]any            `json:"info"`
	Paths   map[string]map[string]any `json:"paths"`
}

func TestOpenAPISpecsAreValid(t *testing.T) {
	require.NotEmpty(t, openapi.RelaySpec)
	require.NotEmpty(t, openapi.APISpec)

	for name, data := range map[string][]byte{
		"relay": openapi.RelaySpec,
		"api":   openapi.APISpec,
	} {
		t.Run(name, func(t *testing.T) {
			var doc openapiDocument
			require.NoError(t, json.Unmarshal(data, &doc))
			assert.Equal(t, "3.0.1", doc.OpenAPI)
			assert.NotEmpty(t, doc.Info["title"])
			assert.NotEmpty(t, doc.Info["version"])
			assert.NotEmpty(t, doc.Paths, "spec must describe at least one endpoint")
		})
	}
}

func TestGetAPIDocs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/docs", GetAPIDocs)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/docs", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Success bool `json:"success"`
		Data    []struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)

	ids := make(map[string]bool, len(resp.Data))
	for _, d := range resp.Data {
		ids[d.ID] = true
		assert.Equal(t, "/api/docs/"+d.ID+".json", d.URL)
	}
	assert.True(t, ids["relay"], "index must list the relay spec")
	assert.True(t, ids["api"], "index must list the management api spec")
}

func TestGetAPIDoc(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/docs/:doc_name", GetAPIDoc)

	t.Run("known doc serves raw spec", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/docs/relay.json", nil)
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))
		var doc openapiDocument
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &doc))
		assert.NotEmpty(t, doc.Paths)
	})

	t.Run("unknown doc returns 404", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/docs/nope.json", nil)
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code)
		var resp struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.False(t, resp.Success)
	})
}
