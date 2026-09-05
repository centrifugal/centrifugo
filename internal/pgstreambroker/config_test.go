package pgstreambroker

import (
	"strings"
	"testing"

	"github.com/centrifugal/centrifuge"
)

// A table prefix reaches DDL unquoted, so one PostgreSQL can't parse has to be
// caught at construction — otherwise it surfaces much later as a syntax error
// naming a generated table rather than the setting behind it.
func TestNewPostgresStreamBroker_RejectsBadTablePrefix(t *testing.T) {
	node, err := centrifuge.New(centrifuge.Config{})
	if err != nil {
		t.Fatalf("centrifuge.New: %v", err)
	}
	t.Cleanup(func() { _ = node.Shutdown(t.Context()) })

	// Rejected before any connection is attempted, so this needs no Postgres.
	_, err = NewPostgresStreamBroker(node, PostgresStreamBrokerConfig{
		DSN:         "postgres://test:test@localhost:5432/test",
		TablePrefix: "cf-prod",
	})
	if err == nil {
		t.Fatal("expected an error for a prefix PostgreSQL cannot parse unquoted")
	}
	if !strings.Contains(err.Error(), "table_prefix") {
		t.Fatalf("error should name the setting at fault, got: %v", err)
	}
}
