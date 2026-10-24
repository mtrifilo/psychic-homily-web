package catalog

import (
	"os"
	"testing"

	"psychic-homily-backend/internal/testutil/netguard"
)

// TestMain also clears the sitemap diet flags, so a shell that exports them
// (a sourced stage env, say) cannot switch rules on under tests that build
// the service through NewSitemapService and expect every rule off.
func TestMain(m *testing.M) {
	for _, key := range sitemapDietEnvKeys {
		_ = os.Unsetenv(key)
	}
	os.Exit(netguard.RefuseNonLoopbackNetwork(m))
}
