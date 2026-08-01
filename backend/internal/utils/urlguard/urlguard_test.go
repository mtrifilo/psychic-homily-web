package urlguard

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

// testResolver is the fixed table the guard resolves against, so no test here
// touches real DNS. The public host is example.com's real address; the hostile
// ones are what an attacker-controlled zone would answer.
var testResolver = MapResolver{
	"images.example.com":   {"93.184.216.34"},
	"cdn.example.com":      {"93.184.216.34", "2606:2800:220:1:248:1893:25c8:1946"},
	"rebind.example.com":   {"169.254.169.254"},
	"loopback.example.com": {"127.0.0.1"},
	"mixed.example.com":    {"93.184.216.34", "10.0.0.5"},
	"ula.example.com":      {"fc00::1"},
}

func testGuard() *Guard { return New(testResolver) }

// TestValidate_RejectsLiteralAddresses mirrors the bypass corpus the edge guard
// carries (frontend/lib/og/remoteImage.test.ts, PSY-1672). Every entry is a URL
// a submitter can type into image_url that reaches an internal address on some
// fetcher, and none of them needs DNS to classify.
func TestValidate_RejectsLiteralAddresses(t *testing.T) {
	cases := []struct{ name, url string }{
		{"loopback", "https://127.0.0.1/x.jpg"},
		{"loopback ipv6", "https://[::1]/x.jpg"},
		{"cloud metadata", "https://169.254.169.254/latest/meta-data/"},
		{"cloud metadata over http", "http://169.254.169.254/latest/meta-data/"},
		{"ipv4-mapped metadata, dotted", "https://[::ffff:169.254.169.254]/x.jpg"},
		{"ipv4-mapped metadata, hex groups", "https://[::ffff:a9fe:a9fe]/x.jpg"},
		{"ipv4-compatible loopback", "https://[::127.0.0.1]/x.jpg"},
		{"nat64 metadata", "https://[64:ff9b::169.254.169.254]/x.jpg"},
		{"6to4 loopback", "https://[2002:7f00:1::]/x.jpg"},
		{"teredo", "https://[2001::1]/x.jpg"},
		{"unique-local ipv6", "https://[fc00::1]/x.jpg"},
		{"link-local ipv6", "https://[fe80::1]/x.jpg"},
		{"link-local ipv6 with zone", "https://[fe80::1%25eth0]/x.jpg"},
		{"rfc1918 10/8", "https://10.0.0.5/x.jpg"},
		{"rfc1918 192.168/16", "https://192.168.1.1/x.jpg"},
		{"rfc1918 172.16/12", "https://172.16.0.1/x.jpg"},
		{"cgnat", "https://100.64.0.1/x.jpg"},
		{"oracle metadata", "https://192.0.0.192/x.jpg"},
		{"unspecified", "https://0.0.0.0/x.jpg"},
		{"broadcast", "https://255.255.255.255/x.jpg"},
		{"decimal loopback", "https://2130706433/x.jpg"},
		{"decimal metadata", "https://2852039166/x.jpg"},
		{"hex loopback", "https://0x7f000001/x.jpg"},
		{"octal loopback", "https://0177.0.0.1/x.jpg"},
		{"short-form loopback", "https://127.1/x.jpg"},
		{"userinfo hiding the host", "https://example.com@169.254.169.254/x.jpg"},
		{"userinfo with password", "https://user:pass@169.254.169.254/x.jpg"},
		{"localhost", "https://localhost/x.jpg"},
		{"localhost, trailing dot", "https://localhost./x.jpg"},
		{"localhost, mixed case", "https://LocalHost/x.jpg"},
		{"localhost subdomain", "https://api.localhost/x.jpg"},
		{"mdns .local", "https://printer.local/x.jpg"},
		{"private .internal namespace", "https://vault.internal/x.jpg"},
		{"gcp metadata by name", "https://metadata.google.internal/computeMetadata/v1/"},
		{"public literal with a private port host", "https://[::ffff:10.0.0.1]:8080/x.jpg"},
	}
	g := testGuard()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := g.Validate(context.Background(), c.url, "Image URL")
			if err == nil {
				t.Fatalf("Validate(%q) = nil, want a rejection", c.url)
			}
			if !strings.Contains(err.Error(), "Image URL") {
				t.Errorf("rejection must name the field, got %q", err)
			}
		})
	}
}

// TestValidate_RejectsUnparseableAndNonHTTP confirms the guard fails closed on
// input it cannot classify, rather than waving it through to a fetcher whose
// URL parser is more permissive than Go's.
func TestValidate_RejectsUnparseableAndNonHTTP(t *testing.T) {
	cases := []struct{ name, url string }{
		{"javascript scheme", "javascript:alert(1)"},
		{"data scheme", "data:image/png;base64,AAAA"},
		{"file scheme", "file:///etc/passwd"},
		{"gopher scheme", "gopher://127.0.0.1:11211/x"},
		{"no host", "https:///x.jpg"},
		{"backslash smuggled userinfo", `https://example.com\@169.254.169.254/x.jpg`},
		{"percent-escaped host", "https://%31%32%37.0.0.1/x.jpg"},
		{"control character in host", "https://example.com\n@169.254.169.254/x.jpg"},
		{"space in host", "https://exam ple.com/x.jpg"},
	}
	g := testGuard()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := g.Validate(context.Background(), c.url, "Image URL"); err == nil {
				t.Fatalf("Validate(%q) = nil, want a rejection", c.url)
			}
		})
	}
}

// TestValidate_ResolvesHostnames is the layer the edge guard cannot have: a
// name that looks ordinary and answers with an internal address.
func TestValidate_ResolvesHostnames(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"public host", "https://images.example.com/flyer.jpg", false},
		{"public host over http", "http://images.example.com/flyer.jpg", false},
		{"public host, dual stack", "https://cdn.example.com/flyer.jpg", false},
		{"public host, trailing dot", "https://images.example.com./flyer.jpg", false},
		{"public host, mixed case", "https://Images.Example.COM/flyer.jpg", false},
		{"name answering with cloud metadata", "https://rebind.example.com/flyer.jpg", true},
		{"name answering with loopback", "https://loopback.example.com/flyer.jpg", true},
		{"name answering with one public and one private address", "https://mixed.example.com/flyer.jpg", true},
		{"name answering with unique-local ipv6", "https://ula.example.com/flyer.jpg", true},
		{"name that does not resolve", "https://nope.example.com/flyer.jpg", true},
	}
	g := testGuard()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := g.Validate(context.Background(), c.url, "Image URL")
			if c.wantErr && err == nil {
				t.Fatalf("Validate(%q) = nil, want a rejection", c.url)
			}
			if !c.wantErr && err != nil {
				t.Fatalf("Validate(%q) = %v, want nil", c.url, err)
			}
		})
	}
}

// TestValidate_AcceptsPublicLiteralsAndEmpty covers what must keep working: a
// public address typed literally, and the empty value callers use to clear the
// field.
func TestValidate_AcceptsPublicLiteralsAndEmpty(t *testing.T) {
	g := testGuard()
	for _, u := range []string{
		"",
		"   ",
		"https://93.184.216.34/x.jpg",
		"https://1.1.1.1./x.jpg",
		"https://[2606:2800:220:1:248:1893:25c8:1946]/x.jpg",
		"https://images.example.com/flyer.jpg?w=800#frag",
	} {
		if err := g.Validate(context.Background(), u, "Image URL"); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", u, err)
		}
	}
}

// TestValidate_DoesNotLeakResolvedAddresses: the message names the host the
// submitter typed and nothing the resolver said, so a write endpoint cannot be
// read as a map of the internal network.
func TestValidate_DoesNotLeakResolvedAddresses(t *testing.T) {
	err := testGuard().Validate(context.Background(), "https://rebind.example.com/x.jpg", "Image URL")
	if err == nil {
		t.Fatal("expected a rejection")
	}
	if strings.Contains(err.Error(), "169.254.169.254") {
		t.Errorf("rejection must not echo the resolved address, got %q", err)
	}
	if !strings.Contains(err.Error(), "rebind.example.com") {
		t.Errorf("rejection should name the submitted host, got %q", err)
	}
}

// TestValidate_ResolverErrorsRejectAndCancelledContextRejects: a lookup that
// cannot complete is a value the guard cannot vouch for, so it is refused
// rather than admitted.
func TestValidate_ResolverErrorsReject(t *testing.T) {
	g := New(errorResolver{})
	if err := g.Validate(context.Background(), "https://images.example.com/x.jpg", "Image URL"); err == nil {
		t.Error("a resolver error must reject, not pass")
	}

	g = New(emptyResolver{})
	if err := g.Validate(context.Background(), "https://images.example.com/x.jpg", "Image URL"); err == nil {
		t.Error("an empty answer must reject, not pass")
	}

	// A caller whose request was cancelled must not end up with a value that
	// was never classified. MapResolver ignores ctx, so this asserts through a
	// resolver that honours it the way *net.Resolver does.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := New(ctxResolver{}).Validate(ctx, "https://images.example.com/x.jpg", "Image URL"); err == nil {
		t.Error("a cancelled context must reject, not pass")
	}
}

// TestUseResolver restores the previous resolver, so a test that swaps the
// package Default cannot leak the stub into the next one.
func TestUseResolver(t *testing.T) {
	g := New(errorResolver{})
	original := g.resolver

	restore := g.UseResolver(testResolver)
	if err := g.Validate(context.Background(), "https://images.example.com/x.jpg", "Image URL"); err != nil {
		t.Fatalf("stub resolver not in effect: %v", err)
	}

	restore()
	if g.resolver != original {
		t.Fatal("restore must reinstate the previous resolver")
	}

	// A nil resolver means "the process resolver", never "no resolution".
	if New(nil).resolver == nil {
		t.Error("New(nil) must install the process resolver")
	}
}

// TestMapResolver covers the stub itself: an unknown host is a not-found error,
// not an empty success that a careless guard could read as "no bad addresses".
func TestMapResolver(t *testing.T) {
	addrs, err := testResolver.LookupIPAddr(context.Background(), "IMAGES.example.com")
	if err != nil || len(addrs) != 1 || !addrs[0].IP.Equal(net.ParseIP("93.184.216.34")) {
		t.Fatalf("LookupIPAddr = %v, %v", addrs, err)
	}
	if _, err := testResolver.LookupIPAddr(context.Background(), "absent.example.com"); err == nil {
		t.Error("an absent host must return an error")
	}
}

type errorResolver struct{}

func (errorResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return nil, &net.DNSError{Err: "server misbehaving", IsTemporary: true}
}

type emptyResolver struct{}

func (emptyResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return nil, nil
}

// ctxResolver honours cancellation the way *net.Resolver does.
type ctxResolver struct{}

func (ctxResolver) LookupIPAddr(ctx context.Context, _ string) ([]net.IPAddr, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(time.Second):
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
}
