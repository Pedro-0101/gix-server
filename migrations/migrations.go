// Package migrations embute os arquivos .sql de schema para o servidor aplicá-los
// no boot (ver internal/store). As migrations são idempotentes (IF NOT EXISTS).
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
