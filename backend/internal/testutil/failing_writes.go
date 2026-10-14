package testutil

import (
	"testing"

	"gorm.io/gorm"
)

// FailUpdates makes every UPDATE of a row in table raise an error until the
// test ends, so a test can drive the path that handles a failed write. The
// raised message is FailedUpdateMessage.
func FailUpdates(t *testing.T, db *gorm.DB, table string) {
	t.Helper()
	if err := db.Exec(`CREATE OR REPLACE FUNCTION testutil_fail_update() RETURNS trigger AS $$
		BEGIN RAISE EXCEPTION '` + FailedUpdateMessage + `'; END $$ LANGUAGE plpgsql`).Error; err != nil {
		t.Fatalf("create failing-update function: %v", err)
	}
	if err := db.Exec(`CREATE TRIGGER testutil_fail_update BEFORE UPDATE ON ` + table +
		` FOR EACH ROW EXECUTE FUNCTION testutil_fail_update()`).Error; err != nil {
		t.Fatalf("create failing-update trigger on %s: %v", table, err)
	}
	t.Cleanup(func() {
		if err := db.Exec(`DROP TRIGGER testutil_fail_update ON ` + table).Error; err != nil {
			t.Errorf("drop failing-update trigger on %s: %v", table, err)
		}
	})
}

// FailedUpdateMessage is the error text an update refused by FailUpdates
// carries.
const FailedUpdateMessage = "update refused by testutil.FailUpdates"
