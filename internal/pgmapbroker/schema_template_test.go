package pgmapbroker

import (
	"regexp"
	"testing"
)

// execSchemaWithRetry re-runs the whole batch on a concurrent-DDL conflict,
// which is only safe while the batch stays one implicit transaction: no
// statement of its own opening or closing a transaction, and nothing that
// cannot run inside a transaction block at all (CREATE INDEX CONCURRENTLY
// would fail outright with 25001, and a retry could not undo the half of the
// batch that already committed). The invariant lives in the SQL template, so
// pin it here rather than in a comment.
func TestSchemaTemplate_StaysOneImplicitTransaction(t *testing.T) {
	concurrently := regexp.MustCompile(`(?i)\bCONCURRENTLY\b`)
	txControl := regexp.MustCompile(`(?im)^\s*(BEGIN|COMMIT|ROLLBACK|START\s+TRANSACTION)\b`)

	for _, tc := range []struct {
		name   string
		prefix string
		binary bool
	}{
		{"jsonb", "cf_map_", false},
		{"binary", "cf_binary_map_", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql := renderSchema(tc.prefix, tc.binary)
			if loc := concurrently.FindString(sql); loc != "" {
				t.Errorf("schema template uses %q — it cannot run inside the implicit transaction execSchemaWithRetry relies on", loc)
			}
			// Only the DDL half is checked for transaction control: the funcs
			// half is dollar-quoted PL/pgSQL, where BEGIN opens a block.
			ddl, _ := splitSchemaSQL(sql)
			if loc := txControl.FindString(ddl); loc != "" {
				t.Errorf("schema DDL contains explicit transaction control %q — the batch must be left to the server's implicit transaction", loc)
			}
		})
	}
}
