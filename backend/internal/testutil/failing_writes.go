package testutil

import (
	"database/sql"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// FailedUpdateMessage is the error text an update refused by FailUpdates
// carries.
const FailedUpdateMessage = "update refused by testutil.FailUpdates"

// FailedInsertMessage is the error text an insert refused by FailInserts
// carries.
const FailedInsertMessage = "insert refused by testutil.FailInserts"

// FailUpdates makes every UPDATE of a row in table raise FailedUpdateMessage
// until the test ends, so a test can drive the path that handles a failed
// write. table is an unquoted identifier from the test itself. One call per
// table and database at a time: a second call on the same table before the
// first test ends fails.
func FailUpdates(t *testing.T, db *gorm.DB, table string) {
	t.Helper()
	failWrites(t, db, table, "UPDATE", FailedUpdateMessage)
}

// FailInserts is FailUpdates for every INSERT into table, raising
// FailedInsertMessage.
func FailInserts(t *testing.T, db *gorm.DB, table string) {
	t.Helper()
	failWrites(t, db, table, "INSERT", FailedInsertMessage)
}

// failWrites installs a BEFORE trigger on table that raises message for every
// row operation writes, and removes it when the test ends.
func failWrites(t *testing.T, db *gorm.DB, table, operation, message string) {
	t.Helper()
	function := "testutil_fail_" + strings.ToLower(operation) + "_" + table
	if err := db.Exec(`CREATE FUNCTION ` + function + `() RETURNS trigger AS $$
		BEGIN RAISE EXCEPTION '` + message + `'; END $$ LANGUAGE plpgsql`).Error; err != nil {
		t.Fatalf("create failing-%s function for %s: %v", operation, table, err)
	}
	if err := db.Exec(`CREATE TRIGGER ` + function + ` BEFORE ` + operation + ` ON ` + table +
		` FOR EACH ROW EXECUTE FUNCTION ` + function + `()`).Error; err != nil {
		t.Fatalf("create failing-%s trigger on %s: %v", operation, table, err)
	}
	t.Cleanup(func() {
		if err := db.Exec(`DROP FUNCTION ` + function + `() CASCADE`).Error; err != nil {
			t.Errorf("drop failing-%s trigger on %s: %v", operation, table, err)
		}
	})
}

// UnavailableDB returns a Postgres GORM handle whose every query fails, as
// one over a database that cannot be reached does, so a test can drive the
// path that handles a failed read. It needs no running database.
func UnavailableDB(t *testing.T) *gorm.DB {
	t.Helper()
	conn, err := sql.Open("pgx", "host=unavailable.invalid")
	if err != nil {
		t.Fatalf("open unavailable database: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close unavailable database: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("wrap unavailable database: %v", err)
	}
	return db
}

// UnansweringDB returns a Postgres GORM handle on a local listener that accepts
// connections and never writes to them, so every query waits until its
// context ends, as one over a connection that stopped answering does. It
// needs no running database.
func UnansweringDB(t *testing.T) *gorm.DB {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for unanswering database: %v", err)
	}

	var mu sync.Mutex
	var accepted []net.Conn
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			accepted = append(accepted, c)
			mu.Unlock()
		}
	}()

	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	conn, err := sql.Open("pgx", "host=127.0.0.1 port="+port+" user=nobody dbname=nothing sslmode=disable")
	if err != nil {
		t.Fatalf("open unanswering database: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
		_ = ln.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range accepted {
			_ = c.Close()
		}
	})
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("wrap unanswering database: %v", err)
	}
	return db
}
