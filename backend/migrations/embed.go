// Package migrations встраивает SQL-миграции goose в бинарник.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
