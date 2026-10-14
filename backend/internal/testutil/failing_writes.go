package testutil

import (
	"testing"

	"gorm.io/gorm"
)

// FailedUpdateMessage is the error text an update refused by FailUpdates
// carries.
const FailedUpdateMessage = "update refused by testutil.FailUpdates"

// FailUpdates makes every UPDATE of a row in table raise FailedUpdateMessage
// until the test ends, so a test can drive the path that handles a failed
// write. table is an unquoted identifier from the test itself. One call per
// table and database at a time: a second call on the same table before the
// first test ends fails.
func FailUpdates(t *testing.T, db *gorm.DB, table string) {
	t.Helper()
	function := "testutil_fail_update_" + table
	if err := db.Exec(`CREATE FUNCTION ` + function + `() RETURNS trigger AS $$
		BEGIN RAISE EXCEPTION '` + FailedUpdateMessage + `'; END $$ LANGUAGE plpgsql`).Error; err != nil {
		t.Fatalf("create failing-update function for %s: %v", table, err)
	}
	if err := db.Exec(`CREATE TRIGGER ` + function + ` BEFORE UPDATE ON ` + table +
		` FOR EACH ROW EXECUTE FUNCTION ` + function + `()`).Error; err != nil {
		t.Fatalf("create failing-update trigger on %s: %v", table, err)
	}
	t.Cleanup(func() {
		if err := db.Exec(`DROP FUNCTION ` + function + `() CASCADE`).Error; err != nil {
			t.Errorf("drop failing-update trigger on %s: %v", table, err)
		}
	})
}
