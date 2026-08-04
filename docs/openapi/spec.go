// Package openapi embeds the OpenAPI specification files that ship with the
// project so they can be served by the API documentation endpoints.
package openapi

import (
	_ "embed"
)

// RelaySpec describes the AI relay API exposed to end users: the
// OpenAI-compatible endpoints under /v1 plus the Claude, Gemini, Midjourney
// and video task formats.
//
//go:embed relay.json
var RelaySpec []byte

// APISpec describes the management API served under /api (users, tokens,
// channels, billing, system settings, etc.).
//
//go:embed api.json
var APISpec []byte
