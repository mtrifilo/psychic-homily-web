package auth

import (
	"time"
)

const (
	// cliCallbackTTL is how long a stored CLI callback stays usable. The
	// cli_callback_id cookie lives exactly as long.
	cliCallbackTTL = 5 * time.Minute

	// cliCallbackCapacity bounds the store. Past it the oldest entry is
	// evicted, so a flood of CLI starts costs the oldest pending CLI sign-in
	// its loopback redirect rather than growing the process's memory.
	cliCallbackCapacity = 1000
)

// cliCallbackStore holds CLI callback URLs by the opaque id parked in the
// cli_callback_id cookie, for the length of one OAuth round trip.
// Process-local: a callback that lands on a restarted process finds nothing
// and falls back to the web flow.
var cliCallbackStore = newExpiringRegistry[string](cliCallbackCapacity, cliCallbackTTL, time.Now)

// storeCLICallback stores callbackURL under id in the process-wide store. Its
// result is put's evictionsToReport.
func storeCLICallback(id, callbackURL string) (evictionsToReport int) {
	return cliCallbackStore.put(id, callbackURL)
}

// takeCLICallback consumes the callback stored under id: single use, so a
// replayed cli_callback_id cookie finds nothing.
func takeCLICallback(id string) (string, bool) {
	return cliCallbackStore.take(id)
}
