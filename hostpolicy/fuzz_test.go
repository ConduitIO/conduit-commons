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
	"net/url"
	"strings"
	"testing"
)

// FuzzCheckEndpoint checks the matching property: CheckEndpoint accepts a URL
// only if it is https, has no user info, and its normalized host and port equal
// the single allowlisted entry. Anything else, including every suffix or
// prefix variation of the entry, is refused.
func FuzzCheckEndpoint(f *testing.F) {
	seeds := [][3]string{
		{"minio.internal", "9000", "https://minio.internal:9000"},
		{"minio.internal", "9000", "https://x.minio.internal:9000"},
		{"s3.amazonaws.com", "", "https://x.execute-api.us-east-1.amazonaws.com"},
		{"s3.amazonaws.com", "", "http://s3.amazonaws.com"},
		{"::1", "9443", "https://[::1]:9443"},
		{"::1", "9443", "https://[::ffff:0:1]:9443"},
		{"host", "", "https://user@host"},
		{"host", "", "https://HOST."},
		{"xn--bcher-kva.example", "", "https://bücher.example"},
	}
	for _, s := range seeds {
		f.Add(s[0], s[1], s[2])
	}
	f.Fuzz(func(t *testing.T, host, port, raw string) {
		e, err := NewEndpoint(host, port)
		if err != nil {
			return
		}
		u, err := url.Parse(raw)
		if err != nil {
			return
		}
		p := Policy{AmbientCredentials: true, AllowEndpoints: []Endpoint{e}}
		if p.CheckEndpoint(u) != nil {
			return
		}
		// Accepted: the property must hold.
		if !strings.EqualFold(u.Scheme, "https") || u.User != nil {
			t.Fatalf("accepted non-https or user-info URL %q", raw)
		}
		h, err := normalizeHost(u.Hostname())
		if err != nil || h != e.Host {
			t.Fatalf("accepted host %q for entry %q", u.Hostname(), e.Host)
		}
		gotPort := u.Port()
		if gotPort == "" {
			gotPort = "443"
		}
		if gotPort != e.Port {
			t.Fatalf("accepted port %q for entry port %q", gotPort, e.Port)
		}
		// And the zero policy must refuse the same URL.
		if (Policy{}).CheckEndpoint(u) == nil {
			t.Fatal("zero policy accepted an endpoint")
		}
	})
}
