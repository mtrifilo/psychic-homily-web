package catalog

import (
	"net"
	"net/http"
	"sync/atomic"
	"syscall"
)

// refusingBandcampTransport fails every request the way a refused TCP dial
// fails, before any HTTP response exists, so a Bandcamp profile resolve never
// leaves the process. It counts the requests it refuses.
type refusingBandcampTransport struct {
	refused atomic.Int32
}

func (f *refusingBandcampTransport) RoundTrip(*http.Request) (*http.Response, error) {
	f.refused.Add(1)
	return nil, &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
}
