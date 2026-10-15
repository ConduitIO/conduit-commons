// Copyright © 2026 Meroxa, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package hostpolicy

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/matryer/is"
)

func reqFor(t *testing.T, raw string) *http.Request {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Request{URL: u}
}

func TestRedirectGuard_Table(t *testing.T) {
	guard := RedirectGuard()
	const orig = "https://bucket.s3.us-east-1.amazonaws.com/key"
	testCases := []struct {
		to   string
		want Code
	}{
		{"https://bucket.s3.us-east-1.amazonaws.com/other", ""},
		{"https://BUCKET.s3.us-east-1.amazonaws.com/other", ""},
		{"https://bucket.s3.us-east-1.amazonaws.com./other", ""},
		{"https://bucket.s3.us-east-1.amazonaws.com:443/other", ""},
		{"https://evil.example/key", CodeRedirectRefused},
		{"https://sub.bucket.s3.us-east-1.amazonaws.com/key", CodeRedirectRefused}, // Go keeps Authorization here
		{"https://bucket.s3.eu-west-1.amazonaws.com/key", CodeRedirectRefused},
		{"http://bucket.s3.us-east-1.amazonaws.com/key", CodeRedirectRefused}, // scheme downgrade
		{"https://bucket.s3.us-east-1.amazonaws.com:8443/key", CodeRedirectRefused},
		{"https://user:pw@bucket.s3.us-east-1.amazonaws.com/key", CodeRedirectRefused},
		{"https://169.254.169.254/latest/meta-data", CodeRedirectRefused},
	}
	for _, tc := range testCases {
		t.Run(tc.to, func(t *testing.T) {
			err := guard(reqFor(t, tc.to), []*http.Request{reqFor(t, orig)})
			assertCode(t, err, tc.want)
			if err != nil && strings.Contains(err.Error(), "/key") {
				t.Fatalf("error must name hosts only, got %q", err)
			}
		})
	}

	t.Run("every hop is checked against the original host", func(t *testing.T) {
		via := []*http.Request{reqFor(t, orig), reqFor(t, "https://bucket.s3.us-east-1.amazonaws.com/2")}
		assertCode(t, guard(reqFor(t, "https://evil.example/"), via), CodeRedirectRefused)
	})
	t.Run("redirect limit is kept", func(t *testing.T) {
		via := make([]*http.Request, maxRedirects)
		for i := range via {
			via[i] = reqFor(t, orig)
		}
		if err := guard(reqFor(t, orig), via); err == nil {
			t.Fatal("expected the redirect limit to apply")
		}
	})
}

// TestRedirectGuard_LeakThroughRealClient proves the guard stops a session
// token from reaching a second host on a 307, and proves the test can see the
// leak: the control run without the guard must observe the token arriving.
func TestRedirectGuard_LeakThroughRealClient(t *testing.T) {
	run := func(t *testing.T, guarded bool) (secondHits int32, gotToken string, firstHits int32, err error) {
		t.Helper()
		var second, first atomic.Int32
		var token atomic.Value
		token.Store("")
		b := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			second.Add(1)
			token.Store(r.Header.Get("X-Amz-Security-Token"))
			w.WriteHeader(http.StatusOK)
		}))
		defer b.Close()
		// Both servers listen on loopback; the client reaches them as
		// a.example.com and b.example.com (names the httptest certificate
		// covers), so the redirect is cross-host and TLS is verified.
		_, bPort, _ := net.SplitHostPort(b.Listener.Addr().String())
		bURL := "https://b.example.com:" + bPort
		a := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			first.Add(1)
			http.Redirect(w, r, bURL+"/stolen", http.StatusTemporaryRedirect)
		}))
		defer a.Close()
		_, aPort, _ := net.SplitHostPort(a.Listener.Addr().String())

		tr := a.Client().Transport.(*http.Transport).Clone() // trusts the httptest CA
		tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			_, port, _ := net.SplitHostPort(addr)
			return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort("127.0.0.1", port))
		}
		c := &http.Client{Transport: tr, Timeout: 5 * time.Second}
		if guarded {
			c.CheckRedirect = RedirectGuard()
		}
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPut, "https://a.example.com:"+aPort+"/obj", nil)
		req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/...")
		req.Header.Set("X-Amz-Security-Token", "session-token-value")
		resp, err := c.Do(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
		return second.Load(), token.Load().(string), first.Load(), err
	}

	t.Run("control: unguarded client leaks the token", func(t *testing.T) {
		is := is.New(t)
		hits, tok, _, err := run(t, false)
		is.NoErr(err)
		is.Equal(hits, int32(1))
		is.Equal(tok, "session-token-value")
	})
	t.Run("guarded client never reaches the second host", func(t *testing.T) {
		is := is.New(t)
		hits, _, first, err := run(t, true)
		is.Equal(hits, int32(0))
		is.Equal(first, int32(1))
		assertCode(t, err, CodeRedirectRefused)
		// net/http wraps the CheckRedirect error in *url.Error; the retryable
		// interface must still be reachable through it.
		var r interface{ RetryableError() bool }
		is.True(errors.As(err, &r))
		is.Equal(r.RetryableError(), false)
	})
}

func TestIsRefusedAddr(t *testing.T) {
	testCases := []struct {
		addr string
		want bool
	}{
		{"169.254.169.254", true},
		{"169.254.170.2", true},  // ECS credential endpoint
		{"169.254.170.23", true}, // EKS Pod Identity agent, IPv4
		{"169.254.0.1", true},
		{"fd00:ec2::254", true},
		{"fd00:ec2::23", true},
		{"::ffff:169.254.169.254", true},
		{"fe80::1", true},
		{"fe80::1%en0", true},
		{"10.0.0.1", false},
		{"192.168.1.10", false},
		{"172.16.0.1", false},
		{"127.0.0.1", false},
		{"::1", false},
		{"fd00:ec2::1", false},
		{"fd12:3456::1", false},
		{"52.216.0.1", false},
	}
	for _, tc := range testCases {
		t.Run(tc.addr, func(t *testing.T) {
			if got := IsRefusedAddr(netip.MustParseAddr(tc.addr)); got != tc.want {
				t.Fatalf("IsRefusedAddr(%s) = %v, want %v", tc.addr, got, tc.want)
			}
		})
	}
}

func TestDialControl(t *testing.T) {
	ctl := DialControl()
	testCases := []struct {
		address string
		want    Code
	}{
		{"169.254.169.254:80", CodeEndpointLinkLocal},
		{"[fd00:ec2::254]:80", CodeEndpointLinkLocal},
		{"[fd00:ec2::23]:80", CodeEndpointLinkLocal},
		{"[::ffff:169.254.169.254]:80", CodeEndpointLinkLocal},
		{"[fe80::1%en0]:443", CodeEndpointLinkLocal},
		{"not-an-ip:443", CodeEndpointLinkLocal},
		{"10.1.2.3:9000", ""},
		{"127.0.0.1:9000", ""},
		{"[::1]:9000", ""},
	}
	for _, tc := range testCases {
		t.Run(tc.address, func(t *testing.T) {
			assertCode(t, ctl("tcp", tc.address, nil), tc.want)
		})
	}
}

// TestDialControl_RealDialer shows the control runs on the resolved address:
// dialing a host name hands Control an IP, and a link-local IP is refused
// before any packet is sent.
func TestDialControl_RealDialer(t *testing.T) {
	is := is.New(t)
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	is.NoErr(err)
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())

	var seen []string
	inner := DialControl()
	d := net.Dialer{Timeout: 2 * time.Second, Control: func(network, address string, c syscall.RawConn) error {
		seen = append(seen, address)
		return inner(network, address, c)
	}}

	conn, err := d.DialContext(context.Background(), "tcp4", "localhost:"+port)
	is.NoErr(err)
	_ = conn.Close()
	is.True(len(seen) > 0)
	for _, a := range seen {
		host, _, _ := net.SplitHostPort(a)
		_, perr := netip.ParseAddr(host)
		is.NoErr(perr) // Control saw an IP, not "localhost"
	}

	_, err = d.DialContext(context.Background(), "tcp", "169.254.169.254:80")
	assertCode(t, err, CodeEndpointLinkLocal)
}
