package shared

import (
	"testing"
	"time"
)

// A submitted feed-read record runs on the process-wide writer.
func TestSubmitCalendarFeedReadWrite_RunsTheWork(t *testing.T) {
	ran := make(chan struct{})
	if !SubmitCalendarFeedReadWrite(func() { close(ran) }) {
		t.Fatal("an idle writer must accept the work")
	}
	select {
	case <-ran:
	case <-time.After(5 * time.Second):
		t.Fatal("the submitted work did not run within 5s")
	}
}
