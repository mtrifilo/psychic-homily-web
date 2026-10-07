package urlguard

import "testing"

// TestDialControl confirms the dial hook refuses non-public addresses and
// permits public ones.
func TestDialControl(t *testing.T) {
	if err := DialControl("tcp", "169.254.169.254:80", nil); err == nil {
		t.Errorf("DialControl must refuse the cloud-metadata address")
	}
	if err := DialControl("tcp", "127.0.0.1:443", nil); err == nil {
		t.Errorf("DialControl must refuse loopback")
	}
	if err := DialControl("tcp", "8.8.8.8:443", nil); err != nil {
		t.Errorf("DialControl must permit a public address, got %v", err)
	}
	if err := DialControl("tcp", "garbage", nil); err == nil {
		t.Errorf("DialControl must reject a malformed address")
	}
}
