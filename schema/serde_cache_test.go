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

package schema

import (
	"bytes"
	"strings"
	"testing"

	"github.com/conduitio/conduit-commons/rabin"
	"github.com/conduitio/conduit-commons/schema/avro"
	"github.com/matryer/is"
)

// collidingAvroPairs are pairs of Avro schemas with the same Rabin fingerprint
// and different content. In each pair the victim declares `string id` and
// `long amount_cents`, and the other (hostile) schema declares `bytes id` and
// `string amount_cents`. The hostile schema carries 40 filler letters in its
// "doc" attribute, which is where the 64 bits needed to match the victim's
// fingerprint were put (the Rabin fingerprint is a linear CRC). The pairs are
// fixed literals, and the test asserts the fingerprints are equal, so a change
// to Fingerprint() cannot silently turn this test into a no-op. There is one
// pair per parse order because the serde cache is shared across subtests and
// a schema that was cached by one subtest would hide a bug in the other.
var collidingAvroPairs = map[string]struct{ victim, hostile string }{
	"CollideVictimFirst": {
		victim:  "{\"type\":\"record\",\"name\":\"CollideVictimFirst\",\"fields\":[{\"name\":\"id\",\"type\":\"string\"},{\"name\":\"amount_cents\",\"type\":\"long\"}]}",
		hostile: "{\"type\":\"record\",\"name\":\"CollideVictimFirst\",\"doc\":\"idaelkgcen`nibai````````````````````````\",\"fields\":[{\"name\":\"id\",\"type\":\"bytes\"},{\"name\":\"amount_cents\",\"type\":\"string\"}]}",
	},
	"CollideEvilFirst": {
		victim:  "{\"type\":\"record\",\"name\":\"CollideEvilFirst\",\"fields\":[{\"name\":\"id\",\"type\":\"string\"},{\"name\":\"amount_cents\",\"type\":\"long\"}]}",
		hostile: "{\"type\":\"record\",\"name\":\"CollideEvilFirst\",\"doc\":\"iohd`fmjnbaklgoi````````````````````````\",\"fields\":[{\"name\":\"id\",\"type\":\"bytes\"},{\"name\":\"amount_cents\",\"type\":\"string\"}]}",
	},
}

// A schema whose Rabin fingerprint is equal to another schema's must still get
// its own Serde. The serde cache is process-wide, so a shared entry would make
// one pipeline decode records with another pipeline's schema (Invariant 6:
// schema handling never silently mangles data).
func TestSchema_Serde_FingerprintCollisionDoesNotShareCacheEntry(t *testing.T) {
	// The cache is shared across tests and subtests, so each order uses its own
	// pair to start from an empty entry.
	for _, victimFirst := range []bool{true, false} {
		name := "CollideEvilFirst"
		if victimFirst {
			name = "CollideVictimFirst"
		}
		t.Run(name, func(t *testing.T) {
			is := is.New(t)

			pair := collidingAvroPairs[name]
			victim := Schema{Subject: "orders", Version: 1, ID: 1, Type: TypeAvro, Bytes: []byte(pair.victim)}
			evil := Schema{Subject: "orders", Version: 2, ID: 2, Type: TypeAvro, Bytes: []byte(pair.hostile)}

			// Precondition: this is a real collision between different schemas.
			is.True(!bytes.Equal(victim.Bytes, evil.Bytes))
			is.Equal(victim.Fingerprint(), evil.Fingerprint())

			wantVictim, err := avro.Parse(victim.Bytes)
			is.NoErr(err)
			wantEvil, err := avro.Parse(evil.Bytes)
			is.NoErr(err)
			is.True(wantVictim.String() != wantEvil.String())
			is.True(strings.Contains(wantEvil.String(), "bytes"))
			is.True(!strings.Contains(wantVictim.String(), "bytes"))

			first, second := victim, evil
			wantFirst, wantSecond := wantVictim, wantEvil
			if !victimFirst {
				first, second = evil, victim
				wantFirst, wantSecond = wantEvil, wantVictim
			}

			gotFirst, err := first.Serde()
			is.NoErr(err)
			is.Equal(gotFirst.String(), wantFirst.String())

			gotSecond, err := second.Serde()
			is.NoErr(err)
			is.Equal(gotSecond.String(), wantSecond.String())

			// The cache still works: the same schema is served from the entry
			// created for it.
			again, err := first.Serde()
			is.NoErr(err)
			is.True(again == gotFirst) // same pointer, not merely an equal value
		})
	}
}

// Fingerprint is public API (registry code uses it); only the cache key moved
// off it.
func TestSchema_Fingerprint_StillRabin(t *testing.T) {
	is := is.New(t)
	b := []byte(`"string"`)
	is.Equal(Schema{Type: TypeAvro, Bytes: b}.Fingerprint(), rabin.Bytes(b))
}

// The cache key covers everything the Serde depends on, and nothing else.
func TestSchema_serdeCacheKey(t *testing.T) {
	is := is.New(t)

	ref := Reference{Name: "a.proto", Subject: "a", Version: 1}
	base := Schema{Type: TypeProtobuf, Bytes: []byte("x"), References: []Reference{ref}}

	same := base
	same.Subject, same.Version, same.ID = "other", 9, 9
	is.Equal(base.serdeCacheKey(), same.serdeCacheKey()) // identity fields are not part of the key

	differs := map[string]Schema{
		"type":       {Type: TypeAvro, Bytes: base.Bytes, References: base.References},
		"bytes":      {Type: base.Type, Bytes: []byte("y"), References: base.References},
		"references": {Type: base.Type, Bytes: base.Bytes, References: []Reference{{Name: "a.proto", Subject: "a", Version: 2}}},
		"no refs":    {Type: base.Type, Bytes: base.Bytes},
		// the boundary between references and bytes is unambiguous
		"bytes shifted into refs": {Type: base.Type, Bytes: append([]byte(referencesKey(base.References)), 'x')},
	}
	for name, s := range differs {
		if s.serdeCacheKey() == base.serdeCacheKey() {
			t.Errorf("%s: key equals the base schema's key", name)
		}
	}
}
