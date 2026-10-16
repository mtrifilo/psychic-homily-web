package revisiondiff

import (
	"os"
	"testing"

	"psychic-homily-backend/internal/testutil"
)

func TestMain(m *testing.M) { os.Exit(testutil.RefuseNonLoopbackNetwork(m)) }
