// Package testlog captures what a unit under test writes to the standard
// logger, so a test can assert on log CONTENT. Test-only: nothing in the server
// binary imports it.
package testlog

import (
	"bytes"
	"log"
	"testing"
)

// Capture redirects the standard logger for the duration of fn and returns
// everything written to it. Both the writer and the flags are restored, and the
// timestamp prefix is suppressed so assertions see only the messages.
//
// The standard logger is process-global, so a test using Capture must not call
// t.Parallel().
func Capture(t *testing.T, fn func()) string {
	t.Helper()

	var buf bytes.Buffer
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	defer func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	}()
	log.SetOutput(&buf)
	log.SetFlags(0)

	fn()
	return buf.String()
}
