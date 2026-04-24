// Command gen-e2e-seed prints the canonical radio seed data as SQL
// INSERT statements to stdout. It's invoked from
// frontend/e2e/setup-db.sh to populate the E2E database without
// duplicating the data from backend/internal/seeddata/radio.go.
//
// Usage:
//
//	go run ./cmd/gen-e2e-seed | psql "$E2E_DB_URL"
//
// Every statement is idempotent (ON CONFLICT (slug) DO NOTHING), so
// running against an already-seeded database is a safe no-op.
package main

import (
	"fmt"
	"os"

	"psychic-homily-backend/internal/seeddata"
)

func main() {
	if err := seeddata.RenderRadioSeedSQL(os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "gen-e2e-seed: %v\n", err)
		os.Exit(1)
	}
}
