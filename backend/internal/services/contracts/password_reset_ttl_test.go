package contracts

import (
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"
)

// frontendPasswordResetPath is the module the reset copy reads the emailed
// link's lifetime from.
const frontendPasswordResetPath = "../../../../frontend/lib/password-reset.ts"

// The reset copy promises the emailed link works for the minutes it states;
// the token must last exactly that long.
func TestPasswordResetTokenTTL_MatchesTheFrontendCopy(t *testing.T) {
	source, err := os.ReadFile(frontendPasswordResetPath)
	if err != nil {
		t.Fatalf("This test pins PasswordResetTokenTTL to the frontend's copy. Reading %s: %v", frontendPasswordResetPath, err)
	}
	match := regexp.MustCompile(`(?m)^export const PASSWORD_RESET_EXPIRY_MINUTES\s*=\s*(\d+)\b`).FindSubmatch(source)
	if match == nil {
		t.Fatalf("no PASSWORD_RESET_EXPIRY_MINUTES number constant found in %s", frontendPasswordResetPath)
	}
	minutes, err := strconv.Atoi(string(match[1]))
	if err != nil {
		t.Fatal(err)
	}
	if got := time.Duration(minutes) * time.Minute; got != PasswordResetTokenTTL {
		t.Errorf("frontend states %v, PasswordResetTokenTTL is %v", got, PasswordResetTokenTTL)
	}
}
