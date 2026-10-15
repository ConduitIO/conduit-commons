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
	"fmt"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

const (
	schemeHTTPS      = "https"
	defaultHTTPSPort = "443"
)

// Policy is the operator's decision about ambient credentials. The zero value
// denies: every Check method on it returns an [Error] with
// [CodeAmbientCredentialsDisabled].
//
// A Policy is a value. [WithPolicy] and [FromContext] copy its slices, so a
// connector cannot change the engine's policy by mutating what it received.
type Policy struct {
	// AmbientCredentials allows connectors to use the process's ambient cloud
	// identity. When false nothing else in the Policy matters.
	AmbientCredentials bool
	// AllowEndpoints are the only custom endpoints that may receive requests
	// signed with ambient credentials. Matching is exact on host and port, https
	// only. When no custom endpoint is configured, a connector uses its cloud
	// SDK's own endpoint resolution, which is not subject to this list.
	AllowEndpoints []Endpoint
	// AllowResources are connector-defined resource names (for S3, bucket
	// names) the ambient identity may touch. Exact match only.
	AllowResources []string
	// AllowProxy is the operator's opt-in to use ambient credentials while an
	// HTTP proxy is configured in the process environment. Under it the
	// link-local refusal in DialControl cannot see the real destination.
	AllowProxy bool
	// Observe, when not nil, is called for every decision a connector reports
	// with [Policy.Report]. The engine uses it for the audit log and metric.
	Observe func(ctx context.Context, d Decision)
}

// Decision is what a connector reports to the engine through [Policy.Report]:
// either that it is using ambient credentials, or that it refused to.
type Decision struct {
	// Outcome is OutcomeUsed or one of the denied outcomes.
	Outcome Outcome
	// Err is the refusal, nil when Outcome is OutcomeUsed.
	Err *Error
	// Region is the cloud region the connector uses, if any.
	Region string
	// EndpointHost is the host requests go to ("" when the cloud SDK's own
	// resolution is used). Never a URL with credentials or query parameters.
	EndpointHost string
}

// Used returns a Decision recording that ambient credentials are in use.
func Used(region, endpointHost string) Decision {
	return Decision{Outcome: OutcomeUsed, Region: region, EndpointHost: endpointHost}
}

// Denied returns a Decision recording a refusal.
func Denied(err *Error) Decision {
	return Decision{Outcome: err.Code.Outcome(), Err: err}
}

// Report passes d to the engine's Observe hook, if there is one.
func (p Policy) Report(ctx context.Context, d Decision) {
	if p.Observe != nil {
		p.Observe(ctx, d)
	}
}

func (p Policy) clone() Policy {
	p.AllowEndpoints = slices.Clone(p.AllowEndpoints)
	p.AllowResources = slices.Clone(p.AllowResources)
	return p
}

type policyCtxKey struct{}

// WithPolicy returns a context carrying p. Only the engine calls this; a value
// set here cannot come from a pipeline config.
func WithPolicy(ctx context.Context, p Policy) context.Context {
	return context.WithValue(ctx, policyCtxKey{}, p.clone())
}

// FromContext returns the policy on ctx, or the zero (deny) Policy when there
// is none.
func FromContext(ctx context.Context) Policy {
	p, ok := ctx.Value(policyCtxKey{}).(Policy)
	if !ok {
		return Policy{}
	}
	return p.clone()
}

func (p Policy) disabled() *Error {
	return NewError(CodeAmbientCredentialsDisabled, "ambient credentials are not enabled on this Conduit instance")
}

// Endpoint is an allowlisted https host and port. Build it with [NewEndpoint].
type Endpoint struct {
	// Host is a lower-case ASCII host name or an IP address, without brackets
	// and without a trailing dot.
	Host string
	// Port is a decimal port number.
	Port string
}

// String returns https://host:port.
func (e Endpoint) String() string {
	host := e.Host
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return "https://" + host + ":" + e.Port
}

// NewEndpoint validates and normalizes an allowlist entry's host and port. An
// empty port means 443. The host must be ASCII (list IDN hosts in their
// punycode form), must not contain a wildcard, must not carry an IPv6 zone,
// must not start with a dot (that reads like a suffix rule, and there is no
// suffix matching), and must not be an address [IsRefusedAddr] refuses: such
// an entry could never be dialed, so listing it is a configuration mistake
// worth failing on.
func NewEndpoint(host, port string) (Endpoint, error) {
	h, err := normalizeHost(host)
	if err != nil {
		return Endpoint{}, err
	}
	if refusedIPHost(h) {
		return Endpoint{}, fmt.Errorf("host %q is a link-local or metadata address, which is never permitted with ambient credentials", host)
	}
	if port == "" {
		port = defaultHTTPSPort
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port {
		return Endpoint{}, fmt.Errorf("invalid port %q", port)
	}
	return Endpoint{Host: h, Port: port}, nil
}

// normalizeHost lower-cases host, strips brackets and one trailing dot, and
// refuses forms that could make two spellings of one host compare unequal or
// two hosts compare equal: non-ASCII, wildcards, IPv6 zones, empty labels.
func normalizeHost(host string) (string, error) {
	h := strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	h = strings.TrimSuffix(strings.ToLower(h), ".")
	switch {
	case h == "":
		return "", fmt.Errorf("empty host")
	case strings.HasPrefix(h, "."):
		return "", fmt.Errorf("host %q starts with a dot; entries are exact hosts, not suffixes", host)
	case strings.ContainsAny(h, "*%/@?#\\ \t"):
		return "", fmt.Errorf("host %q contains a character that is not allowed", host)
	}
	for _, r := range h {
		if r > 0x7e || r < 0x21 {
			return "", fmt.Errorf("host %q is not ASCII; use its punycode form", host)
		}
	}
	if addr, err := netip.ParseAddr(h); err == nil {
		// Canonical text for IP literals, so ::1 and 0:0::1 compare equal.
		return addr.Unmap().String(), nil
	}
	if strings.Contains(h, ":") || strings.Contains(h, "..") {
		return "", fmt.Errorf("host %q is not a valid host name", host)
	}
	return h, nil
}

// CheckEndpoint reports whether requests signed with ambient credentials may
// go to u. u must be https, carry no user info, and match an AllowEndpoints
// entry exactly on host and port (443 when u has none). There is no suffix
// matching. A nil u is refused; a connector that uses its cloud SDK's own
// endpoint resolution does not call CheckEndpoint.
func (p Policy) CheckEndpoint(u *url.URL) error {
	if !p.AmbientCredentials {
		return p.disabled()
	}
	if u == nil {
		return NewError(CodeEndpointNotAllowed, "no endpoint to check")
	}
	if !strings.EqualFold(u.Scheme, schemeHTTPS) {
		return NewError(CodeEndpointNotAllowed,
			fmt.Sprintf("endpoint scheme %q is not https; ambient credentials are only sent over https", u.Scheme))
	}
	if u.User != nil || u.Opaque != "" {
		return NewError(CodeEndpointNotAllowed, "endpoint must be a plain https://host[:port] URL")
	}
	host, err := normalizeHost(u.Hostname())
	if err != nil {
		return NewError(CodeEndpointNotAllowed, "endpoint host is not valid: "+err.Error())
	}
	if refusedIPHost(host) {
		// Refused whatever the allowlist says; DialControl would refuse the
		// connection anyway, this fails earlier and with the precise reason.
		return NewError(CodeEndpointLinkLocal,
			fmt.Sprintf("endpoint %q is a link-local or metadata address, which is never permitted with ambient credentials", host))
	}
	port := u.Port()
	if port == "" {
		port = defaultHTTPSPort
	}
	for _, e := range p.AllowEndpoints {
		// Normalize the entry too, so an Endpoint built without NewEndpoint
		// still matches exactly and never loosely.
		eh, err := normalizeHost(e.Host)
		if err != nil {
			continue
		}
		ep := e.Port
		if ep == "" {
			ep = defaultHTTPSPort
		}
		if eh == host && ep == port {
			return nil
		}
	}
	return NewError(CodeEndpointNotAllowed,
		fmt.Sprintf("endpoint host %q (port %s) is not on the ambient-credentials allowlist", host, port))
}

// reservedResourceSuffixes are S3 bucket-parameter forms that name something
// other than a plain bucket: access-point aliases (-s3alias), Object Lambda
// (--ol-s3), Outposts access-point aliases (--op-s3), directory buckets
// (--x-s3) and multi-region access points (.mrap). They can resolve to a
// resource in another account, which a name allowlist cannot reason about.
// The list comes from AWS naming documentation, not SDK code.
var reservedResourceSuffixes = []string{"-s3alias", "--ol-s3", "--op-s3", "--x-s3", ".mrap"}

// IsReservedResource reports whether name is an ARN or an alias form that
// [Policy.CheckResource] always refuses. The engine uses it to refuse such
// entries in the allowlist itself at startup.
func IsReservedResource(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if strings.HasPrefix(n, "arn:") {
		return true
	}
	for _, s := range reservedResourceSuffixes {
		if strings.HasSuffix(n, s) {
			return true
		}
	}
	return false
}

// CheckResource reports whether the ambient identity may touch the resource
// name. Only an exact AllowResources entry passes. ARNs, alias forms (see
// [IsReservedResource]) and names containing a wildcard are refused whatever
// the allowlist says.
func (p Policy) CheckResource(name string) error {
	if !p.AmbientCredentials {
		return p.disabled()
	}
	switch {
	case name == "":
		return NewError(CodeResourceNotAllowed, "resource name is empty")
	case strings.Contains(name, "*"):
		return NewError(CodeResourceNotAllowed, fmt.Sprintf("resource %q contains a wildcard", name))
	case IsReservedResource(name):
		return NewError(CodeResourceNotAllowed,
			fmt.Sprintf("resource %q is an ARN or alias form, which ambient credentials never permit; use a plain name", name))
	}
	for _, r := range p.AllowResources {
		if r == name {
			return nil
		}
	}
	return NewError(CodeResourceNotAllowed, fmt.Sprintf("resource %q is not on the ambient-credentials allowlist", name))
}

// ProxyEnvVars are the environment variables Go's http.ProxyFromEnvironment
// (and so most cloud SDK transports) reads to route requests through a proxy.
var ProxyEnvVars = []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"}

// SetProxyEnvVars returns the names of the proxy variables that are set to a
// non-empty value according to env (pass os.Getenv). Values are never
// returned: a proxy URL can carry credentials.
func SetProxyEnvVars(env func(string) string) []string {
	var set []string
	for _, k := range ProxyEnvVars {
		if env(k) != "" {
			set = append(set, k)
		}
	}
	return set
}

// CheckProxy refuses when a proxy variable is set and the operator has not
// opted in with AllowProxy. The reason: a dial-time check sees only the
// proxy's address, so it cannot protect the real destination.
func (p Policy) CheckProxy(env func(string) string) error {
	if !p.AmbientCredentials {
		return p.disabled()
	}
	if p.AllowProxy {
		return nil
	}
	if set := SetProxyEnvVars(env); len(set) > 0 {
		return NewError(CodeProxyNotAllowed,
			fmt.Sprintf("proxy variables are set in the process environment (%s) and the operator has not set allow-proxy",
				strings.Join(set, ", ")))
	}
	return nil
}

// refusedIPHost reports whether a normalized host is an IP literal that
// IsRefusedAddr refuses.
func refusedIPHost(host string) bool {
	addr, err := netip.ParseAddr(host)
	return err == nil && IsRefusedAddr(addr)
}
