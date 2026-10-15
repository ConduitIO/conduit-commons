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
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
)

// maxRedirects matches net/http's default policy, which a custom
// CheckRedirect replaces and must therefore re-implement.
const maxRedirects = 10

// RedirectGuard returns an http.Client CheckRedirect function that refuses
// every redirect to a host or port other than the original request's, every
// redirect to a non-https URL, and every redirect URL carrying user info.
//
// Why: net/http strips Authorization on a cross-host redirect but copies other
// headers, including X-Amz-Security-Token, and keeps Authorization on a
// redirect to a subdomain. This guard is stricter than both: a signed request
// never leaves the host it was signed for.
//
// The returned error is an [*Error] with [CodeRedirectRefused]. It reports
// itself as not retryable (see [Error]). The message names hosts only, never
// the full URL, which can carry presigned query parameters.
func RedirectGuard() func(req *http.Request, via []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) == 0 {
			return nil
		}
		if len(via) >= maxRedirects {
			return errors.New("stopped after 10 redirects")
		}
		orig := via[0].URL
		to := req.URL
		if !strings.EqualFold(to.Scheme, schemeHTTPS) {
			return NewError(CodeRedirectRefused,
				fmt.Sprintf("refused redirect from %q to a %q URL; only https is permitted", orig.Host, to.Scheme))
		}
		if to.User != nil {
			return NewError(CodeRedirectRefused, fmt.Sprintf("refused redirect from %q to a URL with user info", orig.Host))
		}
		if !sameHostPort(orig, to) {
			return NewError(CodeRedirectRefused,
				fmt.Sprintf("refused redirect from host %q to a different host %q", orig.Host, to.Host))
		}
		return nil
	}
}

func sameHostPort(a, b *url.URL) bool {
	ah, aerr := normalizeHost(a.Hostname())
	bh, berr := normalizeHost(b.Hostname())
	if aerr != nil || berr != nil || ah != bh {
		return false
	}
	return effectivePort(a) == effectivePort(b)
}

func effectivePort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if strings.EqualFold(u.Scheme, "http") {
		return "80"
	}
	return defaultHTTPSPort
}

var (
	// imdsV6 is the EC2 instance metadata service over IPv6.
	imdsV6 = netip.MustParseAddr("fd00:ec2::254")
	// podIdentityV6 is the EKS Pod Identity agent over IPv6.
	podIdentityV6 = netip.MustParseAddr("fd00:ec2::23")
)

// nat64Prefix is the well-known NAT64 prefix (RFC 6052). On an IPv6-only
// network with DNS64, 64:ff9b::a9fe:a9fe reaches 169.254.169.254.
var nat64Prefix = netip.MustParsePrefix("64:ff9b::/96")

// IsRefusedAddr reports whether addr is an address DialControl refuses:
//
//   - IPv4 link-local 169.254.0.0/16. On AWS that holds IMDS
//     (169.254.169.254), the ECS credential endpoint (169.254.170.2) and the
//     EKS Pod Identity agent (169.254.170.23); GCP, Azure and OCI serve
//     their metadata on 169.254.169.254 as well.
//   - IPv6 link-local fe80::/10.
//   - The AWS IPv6 metadata addresses fd00:ec2::254 (IMDS) and fd00:ec2::23
//     (Pod Identity).
//   - The IPv4-mapped (::ffff:169.254.x.x) and NAT64-embedded
//     (64:ff9b::169.254.x.x) forms of the IPv4 range.
//
// Scope: this is not a list of every cloud's metadata service. Metadata
// endpoints outside link-local space, such as Alibaba Cloud's
// 100.100.100.200, are not refused. Private ranges and loopback are not
// refused either: an operator who allowlists https://minio.internal chose a
// private address on purpose.
func IsRefusedAddr(addr netip.Addr) bool {
	addr = addr.WithZone("").Unmap()
	if addr.Is6() && nat64Prefix.Contains(addr) {
		b := addr.As16()
		addr = netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]})
	}
	if addr.IsLinkLocalUnicast() {
		return true
	}
	return addr == imdsV6 || addr == podIdentityV6
}

// DialControl returns a net.Dialer Control function that refuses connections
// to the addresses [IsRefusedAddr] lists. It runs on the address actually
// dialed, after DNS resolution, so a host name that resolves (or rebinds) to
// 169.254.169.254 is refused too.
//
// Install it on the dialer of the client that sends data-path requests only,
// not on the client a cloud SDK uses to fetch credentials: the instance-role
// and container credential providers talk to exactly these addresses.
//
// Under a proxy the dialed address is the proxy's, so this check cannot see
// the real destination; that is why ambient credentials with a proxy need the
// operator's allow-proxy opt-in.
//
// The returned error is an [*Error] with [CodeEndpointLinkLocal]. An address
// that cannot be parsed is refused too.
func DialControl() func(network, address string, c syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			host = address
		}
		addr, err := netip.ParseAddr(host)
		if err != nil {
			return NewError(CodeEndpointLinkLocal, fmt.Sprintf("refused to dial unparseable address %q", address))
		}
		if IsRefusedAddr(addr) {
			return NewError(CodeEndpointLinkLocal,
				fmt.Sprintf("refused to dial %s: link-local and metadata addresses are never permitted with ambient credentials", addr))
		}
		return nil
	}
}
