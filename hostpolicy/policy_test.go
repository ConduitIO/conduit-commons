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
	"net/url"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func mustEndpoint(t *testing.T, host, port string) Endpoint {
	t.Helper()
	e, err := NewEndpoint(host, port)
	if err != nil {
		t.Fatalf("NewEndpoint(%q, %q): %v", host, port, err)
	}
	return e
}

func assertCode(t *testing.T, err error, want Code) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		return
	}
	he, ok := AsError(err)
	if !ok {
		t.Fatalf("expected *Error with code %q, got %v", want, err)
	}
	if he.Code != want {
		t.Fatalf("expected code %q, got %q (%v)", want, he.Code, err)
	}
	if he.Suggestion == "" {
		t.Fatalf("code %q has no suggestion", he.Code)
	}
}

func TestFromContext_AbsentMeansDeny(t *testing.T) {
	is := is.New(t)
	p := FromContext(context.Background())
	is.Equal(p.AmbientCredentials, false)

	u, _ := url.Parse("https://s3.amazonaws.com")
	assertCode(t, p.CheckEndpoint(u), CodeAmbientCredentialsDisabled)
	assertCode(t, p.CheckResource("bucket"), CodeAmbientCredentialsDisabled)
	assertCode(t, p.CheckProxy(func(string) string { return "" }), CodeAmbientCredentialsDisabled)
	p.Report(context.Background(), Used("us-east-1", "")) // nil Observe must not panic
}

func TestFromContext_ForeignKeyCannotSpoof(t *testing.T) {
	is := is.New(t)
	type policyCtxKey struct{} // same name, different type
	ctx := context.WithValue(context.Background(), policyCtxKey{}, Policy{AmbientCredentials: true})
	is.Equal(FromContext(ctx).AmbientCredentials, false)
}

func TestWithPolicy_CopiesSlices(t *testing.T) {
	is := is.New(t)
	orig := Policy{
		AmbientCredentials: true,
		AllowResources:     []string{"a"},
		AllowEndpoints:     []Endpoint{mustEndpoint(t, "minio.internal", "9000")},
	}
	ctx := WithPolicy(context.Background(), orig)

	got := FromContext(ctx)
	got.AllowResources[0] = "evil"
	got.AllowEndpoints[0].Host = "evil.example"

	is.Equal(orig.AllowResources[0], "a")
	again := FromContext(ctx)
	is.Equal(again.AllowResources[0], "a")
	is.Equal(again.AllowEndpoints[0].Host, "minio.internal")
}

func TestNewEndpoint(t *testing.T) {
	testCases := []struct {
		host, port string
		want       Endpoint
		wantErr    bool
	}{
		{host: "minio.internal", want: Endpoint{"minio.internal", "443"}},
		{host: "MinIO.Internal.", port: "9000", want: Endpoint{"minio.internal", "9000"}},
		{host: "[::1]", port: "9000", want: Endpoint{"::1", "9000"}},
		{host: "0:0::1", want: Endpoint{"::1", "443"}},
		{host: "::ffff:10.0.0.1", want: Endpoint{"10.0.0.1", "443"}},
		{host: "xn--bcher-kva.example", want: Endpoint{"xn--bcher-kva.example", "443"}},
		{host: "bücher.example", wantErr: true},
		{host: "*.amazonaws.com", wantErr: true},
		{host: "fe80::1%en0", wantErr: true},
		{host: "", wantErr: true},
		{host: "a..b", wantErr: true},
		{host: "user@host", wantErr: true},
		{host: "host", port: "0", wantErr: true},
		{host: "host", port: "65536", wantErr: true},
		{host: "host", port: "0443", wantErr: true},
		{host: "host", port: "https", wantErr: true},
	}
	for _, tc := range testCases {
		t.Run(tc.host+":"+tc.port, func(t *testing.T) {
			got, err := NewEndpoint(tc.host, tc.port)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestCheckEndpoint(t *testing.T) {
	p := Policy{
		AmbientCredentials: true,
		AllowEndpoints: []Endpoint{
			mustEndpoint(t, "minio.internal", "9000"),
			mustEndpoint(t, "s3.us-east-1.amazonaws.com", ""),
			mustEndpoint(t, "::1", "9443"),
			mustEndpoint(t, "xn--bcher-kva.example", ""),
		},
	}
	testCases := []struct {
		url  string
		want Code
	}{
		{"https://minio.internal:9000", ""},
		{"https://MINIO.internal:9000/some/path", ""},
		{"https://minio.internal.:9000", ""},
		{"https://s3.us-east-1.amazonaws.com", ""},
		{"https://s3.us-east-1.amazonaws.com:443", ""},
		{"https://[::1]:9443", ""},
		{"https://[0:0::1]:9443", ""},
		{"https://xn--bcher-kva.example", ""},

		// The design's mandatory regression: a tenant-controlled host under an
		// AWS suffix is refused. So are ELB and EC2 public DNS names.
		{"https://x.execute-api.us-east-1.amazonaws.com", CodeEndpointNotAllowed},
		{"https://my-lb-1234567890.us-east-1.elb.amazonaws.com", CodeEndpointNotAllowed},
		{"https://ec2-203-0-113-25.compute-1.amazonaws.com", CodeEndpointNotAllowed},
		{"https://evil.s3.us-east-1.amazonaws.com", CodeEndpointNotAllowed},
		{"https://s3.us-east-1.amazonaws.com.evil.example", CodeEndpointNotAllowed},

		{"http://minio.internal:9000", CodeEndpointNotAllowed},
		{"http://s3.us-east-1.amazonaws.com", CodeEndpointNotAllowed},
		{"https://minio.internal", CodeEndpointNotAllowed},      // port 443 is not 9000
		{"https://minio.internal:9001", CodeEndpointNotAllowed}, // wrong port
		{"https://user:pw@minio.internal:9000", CodeEndpointNotAllowed},
		{"https://bücher.example", CodeEndpointNotAllowed}, // IDN must be listed as punycode
		{"https://169.254.169.254", CodeEndpointNotAllowed},
		{"https://minio.internal%2eevil:9000", CodeEndpointNotAllowed},
	}
	for _, tc := range testCases {
		t.Run(tc.url, func(t *testing.T) {
			u, err := url.Parse(tc.url)
			if err != nil {
				// An unparseable URL never reaches CheckEndpoint in practice;
				// treat it as refused.
				if tc.want == "" {
					t.Fatal(err)
				}
				return
			}
			assertCode(t, p.CheckEndpoint(u), tc.want)
		})
	}

	t.Run("nil url", func(t *testing.T) {
		assertCode(t, p.CheckEndpoint(nil), CodeEndpointNotAllowed)
	})
	t.Run("empty allowlist refuses everything", func(t *testing.T) {
		u, _ := url.Parse("https://s3.us-east-1.amazonaws.com")
		assertCode(t, Policy{AmbientCredentials: true}.CheckEndpoint(u), CodeEndpointNotAllowed)
	})
	t.Run("unnormalized entry matches exactly, never loosely", func(t *testing.T) {
		raw := Policy{AmbientCredentials: true, AllowEndpoints: []Endpoint{{Host: "MinIO.Internal.", Port: "9000"}}}
		u, _ := url.Parse("https://minio.internal:9000")
		assertCode(t, raw.CheckEndpoint(u), "")
		u, _ = url.Parse("https://x.minio.internal:9000")
		assertCode(t, raw.CheckEndpoint(u), CodeEndpointNotAllowed)
	})
}

func TestCheckResource(t *testing.T) {
	p := Policy{AmbientCredentials: true, AllowResources: []string{"acme-ingest-prod", "x-s3alias"}}
	testCases := []struct {
		name string
		want Code
	}{
		{"acme-ingest-prod", ""},
		{"acme-ingest-staging", CodeResourceNotAllowed},
		{"ACME-INGEST-PROD", CodeResourceNotAllowed},
		{"acme-ingest-*", CodeResourceNotAllowed},
		{"", CodeResourceNotAllowed},
		{"arn:aws:s3:us-east-1:123456789012:accesspoint/ap", CodeResourceNotAllowed},
		{"arn:aws:s3-outposts:us-east-1:123456789012:outpost/op-1/accesspoint/ap", CodeResourceNotAllowed},
		{"ARN:aws:s3:::bucket", CodeResourceNotAllowed},
		// Reserved alias forms are refused even when listed.
		{"x-s3alias", CodeResourceNotAllowed},
		{"x--ol-s3", CodeResourceNotAllowed},
		{"x--op-s3", CodeResourceNotAllowed},
		{"name--usw2-az1--x-s3", CodeResourceNotAllowed},
		{"mfzwi23gnjvgw.mrap", CodeResourceNotAllowed},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assertCode(t, p.CheckResource(tc.name), tc.want)
		})
	}
}

func TestCheckProxy(t *testing.T) {
	for _, k := range ProxyEnvVars {
		t.Run(k, func(t *testing.T) {
			is := is.New(t)
			env := func(name string) string {
				if name == k {
					return "http://user:secret@proxy.example:3128"
				}
				return ""
			}
			err := Policy{AmbientCredentials: true}.CheckProxy(env)
			assertCode(t, err, CodeProxyNotAllowed)
			is.True(strings.Contains(err.Error(), k))
			is.True(!strings.Contains(err.Error(), "secret")) // values never leak

			assertCode(t, Policy{AmbientCredentials: true, AllowProxy: true}.CheckProxy(env), "")
		})
	}
	t.Run("no proxy", func(t *testing.T) {
		assertCode(t, Policy{AmbientCredentials: true}.CheckProxy(func(string) string { return "" }), "")
	})
}

func TestCodes_OutcomeAndSuggestion(t *testing.T) {
	is := is.New(t)
	seen := map[Code]bool{}
	for _, c := range Codes() {
		is.True(!seen[c])
		seen[c] = true
		is.True(strings.HasPrefix(string(c), "connector."))
		is.True(defaultSuggestions[c] != "")
		if c != CodeCredentialsIncomplete {
			is.True(strings.HasPrefix(string(c.Outcome()), "denied_"))
		}
	}
	is.Equal(CodeCredentialsIncomplete.Outcome(), Outcome(""))
}

func TestError(t *testing.T) {
	is := is.New(t)
	e := NewError(CodeEndpointNotAllowed, "nope")
	at := e.At("aws.endpoint")
	is.Equal(e.ConfigPath, "") // At copies
	is.Equal(at.Error(), "connector.endpoint_not_allowed: aws.endpoint: nope")
	is.Equal(at.RetryableError(), false)

	wrapped := fmt.Errorf("config invalid: %w", at)
	got, ok := AsError(wrapped)
	is.True(ok)
	is.Equal(got.Code, CodeEndpointNotAllowed)
}

func TestReport(t *testing.T) {
	is := is.New(t)
	var got []Decision
	p := Policy{AmbientCredentials: true, Observe: func(_ context.Context, d Decision) { got = append(got, d) }}
	ctx := WithPolicy(context.Background(), p)
	pp := FromContext(ctx)
	pp.Report(ctx, Used("eu-west-1", "minio.internal"))
	pp.Report(ctx, Denied(NewError(CodeResourceNotAllowed, "x")))
	is.Equal(len(got), 2)
	is.Equal(got[0].Outcome, OutcomeUsed)
	is.Equal(got[1].Outcome, OutcomeDeniedResource)
}
