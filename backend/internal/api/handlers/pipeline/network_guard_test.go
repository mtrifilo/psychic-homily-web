package pipeline

import (
	"os"
	"testing"

	"psychic-homily-backend/internal/testutil/netguard"
)

func TestMain(m *testing.M) { os.Exit(netguard.RefuseNonLoopbackNetwork(m)) }
