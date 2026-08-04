package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/docs/openapi"

	"github.com/gin-gonic/gin"
)

type openapiDoc struct {
	id   string
	spec []byte
}

// openapiDocs lists the OpenAPI specifications served by the documentation
// endpoints. The order defines the order shown in the frontend docs page.
var openapiDocs = []openapiDoc{
	{id: "relay", spec: openapi.RelaySpec},
	{id: "api", spec: openapi.APISpec},
}

// GetAPIDocs returns the index of available OpenAPI specifications.
func GetAPIDocs(c *gin.Context) {
	docs := make([]gin.H, 0, len(openapiDocs))
	for _, doc := range openapiDocs {
		docs = append(docs, gin.H{
			"id":  doc.id,
			"url": "/api/docs/" + doc.id + ".json",
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    docs,
	})
}

// GetAPIDoc serves the raw OpenAPI specification for a known doc id, e.g.
// GET /api/docs/relay.json. Unknown ids return 404.
func GetAPIDoc(c *gin.Context) {
	name := strings.TrimSuffix(c.Param("doc_name"), ".json")
	for _, doc := range openapiDocs {
		if doc.id == name {
			c.Data(http.StatusOK, "application/json; charset=utf-8", doc.spec)
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{
		"success": false,
		"message": "documentation not found",
	})
}
