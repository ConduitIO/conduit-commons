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

package avro

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/conduitio/conduit-commons/opencdc"
)

// This file holds property tests for Avro schema inference (SerdeForType):
//
//   - Determinism: inferring the schema of the same logical value is a pure
//     function of its shape, not of Go's randomized map iteration order. Either
//     every run fails with the same error or every run succeeds with
//     byte-identical schema text (conduit-commons#292, invariant 6).
//   - Compatibility: for shapes that inferred successfully before the fix, the
//     schema is byte-identical to what it was (testdata/inference_golden.json,
//     generated from the pre-fix code).

type specKind int

const (
	kindInt specKind = iota
	kindInt32
	kindString
	kindBool
	kindFloat64
	kindFloat32
	kindBytes
	kindNil
	kindTime
	kindDuration
	kindStructured // opencdc.StructuredData
	kindMap        // map[string]any
	kindSlice      // []any
)

// valueSpec describes the shape of a value without holding any Go map, so
// instantiate can build a fresh value (and a fresh map iteration order) from
// it any number of times.
type valueSpec struct {
	kind  specKind
	names []string    // keys, for kindStructured and kindMap
	elems []valueSpec // values (same index as names) or items
}

var specKeyPool = []string{"a", "b", "c", "d", "id", "name", "x", "y", "z", "ts"}

// genSpec builds a random shape. Containers often clone an earlier sibling so
// that several structurally identical records meet in one map or slice, which
// is the case that exercises union deduplication.
func genSpec(r *rand.Rand, depth int) valueSpec {
	if depth >= 4 || r.IntN(10) < 4 {
		return valueSpec{kind: specKind(r.IntN(int(kindDuration) + 1))}
	}
	var s valueSpec
	switch r.IntN(3) {
	case 0:
		s.kind = kindStructured
	case 1:
		s.kind = kindMap
	default:
		s.kind = kindSlice
	}
	n := 1 + r.IntN(5)
	if s.kind == kindSlice {
		n = r.IntN(4)
	}
	keys := r.Perm(len(specKeyPool))
	for i := range n {
		var e valueSpec
		if i > 0 && r.IntN(2) == 0 {
			e = s.elems[r.IntN(i)] // clone an earlier sibling
		} else {
			e = genSpec(r, depth+1)
		}
		if s.kind != kindSlice {
			s.names = append(s.names, specKeyPool[keys[i%len(keys)]]+fmt.Sprint(i/len(keys)))
		}
		s.elems = append(s.elems, e)
	}
	return s
}

var specTime = time.Date(2026, 10, 12, 1, 2, 3, 0, time.UTC)

func (s valueSpec) instantiate() any {
	switch s.kind {
	case kindInt:
		return 1
	case kindInt32:
		return int32(2)
	case kindString:
		return "s"
	case kindBool:
		return true
	case kindFloat64:
		return 1.5
	case kindFloat32:
		return float32(2.5)
	case kindBytes:
		return []byte("b")
	case kindNil:
		return nil
	case kindTime:
		return specTime
	case kindDuration:
		return time.Second
	case kindStructured:
		m := make(opencdc.StructuredData, len(s.elems))
		for i, e := range s.elems {
			m[s.names[i]] = e.instantiate()
		}
		return m
	case kindMap:
		m := make(map[string]any, len(s.elems))
		for i, e := range s.elems {
			m[s.names[i]] = e.instantiate()
		}
		return m
	default: // kindSlice
		l := make([]any, len(s.elems))
		for i, e := range s.elems {
			l[i] = e.instantiate()
		}
		return l
	}
}

// outcome is the result of one inference run.
type outcome struct {
	schema string
	err    string
}

func inferOnce(v any) outcome {
	s, err := SerdeForType(v)
	if err != nil {
		return outcome{err: err.Error()}
	}
	return outcome{schema: s.String()}
}

func schemaHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func propertyShapeCount() int {
	if os.Getenv("CONDUIT_PROPERTY_LONG") != "" {
		return 20000
	}
	return 600
}

const (
	propertySeed   = 292
	propertyRepeat = 40 // inference runs per shape; each gets a fresh map order
)

func shapeForIndex(i int) valueSpec {
	return genSpec(rand.New(rand.NewPCG(propertySeed, uint64(i))), 0)
}

// goldenFile maps a shape index to the SHA-256 of the schema text the pre-fix
// code inferred for it whenever it succeeded. Hashes keep the file small; the
// pre-fix code in git history reproduces the text.
const goldenFile = "testdata/inference_golden.json"

func TestSerdeForType_PropertyDeterministic(t *testing.T) {
	n := propertyShapeCount()
	var mixed, multi, allErr, allOK int
	for i := range n {
		spec := shapeForIndex(i)
		results := map[outcome]int{}
		for range propertyRepeat {
			results[inferOnce(spec.instantiate())]++
		}
		var oks, errs []outcome
		for o := range results {
			if o.err != "" {
				errs = append(errs, o)
			} else {
				oks = append(oks, o)
			}
		}
		switch {
		case len(oks) > 0 && len(errs) > 0:
			mixed++
			t.Errorf("shape %d: %d distinct successes and %d distinct errors across %d runs, e.g. error %q",
				i, len(oks), len(errs), propertyRepeat, errs[0].err)
		case len(oks) > 1:
			multi++
			t.Errorf("shape %d: %d different schemas across %d successful runs:\n%s\n%s", i, len(oks), propertyRepeat, oks[0].schema, oks[1].schema)
		case len(errs) > 1:
			t.Errorf("shape %d: %d different errors across %d failing runs: %q / %q", i, len(errs), propertyRepeat, errs[0].err, errs[1].err)
		case len(oks) == 1:
			allOK++
		default:
			allErr++
		}
	}
	t.Logf("%d shapes x %d runs: %d always succeed, %d always fail, %d nondeterministic outcome, %d multiple schemas", n, propertyRepeat, allOK, allErr, mixed, multi)
}

// TestSerdeForType_PropertyCompatibleWithGolden asserts that every shape that
// inferred a schema before the determinism fix still infers byte-identical
// schema text. Regenerate (only ever from the pre-fix code, to keep it a record
// of old behaviour) with CONDUIT_UPDATE_GOLDEN=1.
func TestSerdeForType_PropertyCompatibleWithGolden(t *testing.T) {
	const goldenShapes = 2000
	if os.Getenv("CONDUIT_UPDATE_GOLDEN") != "" {
		golden := map[int]string{}
		for i := range goldenShapes {
			spec := shapeForIndex(i)
			// Take the schema the old code produced when it succeeded. Old
			// code is nondeterministic, so also require that all successful
			// runs agreed; a disagreement is itself a finding.
			var first string
			for range propertyRepeat {
				o := inferOnce(spec.instantiate())
				if o.err != "" {
					continue
				}
				if first != "" && first != o.schema {
					t.Fatalf("shape %d: successful runs disagree:\n%s\n%s", i, first, o.schema)
				}
				first = o.schema
			}
			if first != "" {
				golden[i] = schemaHash(first)
			}
		}
		b, err := json.MarshalIndent(golden, "", " ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenFile, append(b, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %d golden schemas out of %d shapes", len(golden), goldenShapes)
		return
	}

	b, err := os.ReadFile(goldenFile)
	if err != nil {
		t.Fatal(err)
	}
	var golden map[int]string
	if err := json.Unmarshal(b, &golden); err != nil {
		t.Fatal(err)
	}
	keys := make([]int, 0, len(golden))
	for k := range golden {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	for _, i := range keys {
		spec := shapeForIndex(i)
		for range 5 {
			o := inferOnce(spec.instantiate())
			if o.err != "" {
				t.Errorf("shape %d: schema used to infer, now fails: %s", i, o.err)
				break
			}
			if schemaHash(o.schema) != golden[i] {
				t.Errorf("shape %d: schema changed (was sha256 %s)\nnow: %s", i, golden[i], o.schema)
				break
			}
		}
	}
	t.Logf("checked %d golden shapes", len(keys))
}
