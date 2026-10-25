package contracts

import (
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"
)

// frontendMagicLinkPath is the module the auth page's sent-state copy reads
// the emailed link's lifetime from.
const frontendMagicLinkPath = "../../../../frontend/lib/magic-link.ts"

// The sent-state copy promises the emailed link works for the minutes it
// states; the token must last exactly that long.
func TestMagicLinkTokenTTL_MatchesTheFrontendCopy(t *testing.T) {
	source, err := os.ReadFile(frontendMagicLinkPath)
	if err != nil {
		t.Fatalf("This test pins MagicLinkTokenTTL to the frontend's copy. Reading %s: %v", frontendMagicLinkPath, err)
	}
	match := regexp.MustCompile(`(?m)^export const MAGIC_LINK_EXPIRY_MINUTES\s*=\s*(\d+)\b`).FindSubmatch(source)
	if match == nil {
		t.Fatalf("no MAGIC_LINK_EXPIRY_MINUTES number constant found in %s", frontendMagicLinkPath)
	}
	minutes, err := strconv.Atoi(string(match[1]))
	if err != nil {
		t.Fatal(err)
	}
	if got := time.Duration(minutes) * time.Minute; got != MagicLinkTokenTTL {
		t.Errorf("frontend states %v, MagicLinkTokenTTL is %v", got, MagicLinkTokenTTL)
	}
}
