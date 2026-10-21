// Package api встраивает спецификацию OpenAPI в бинарник.
package api

import _ "embed"

//go:embed openapi.yaml
var OpenAPI []byte
