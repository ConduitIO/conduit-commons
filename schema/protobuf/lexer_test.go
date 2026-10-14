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

package protobuf

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"testing"
	"time"
)

// pathological3.proto is protocompile's own fuzz-found input: 669 KB of runs
// of ';' split by vertical tabs, from github.com/bufbuild/protocompile
// parser/testdata, added by upstream commit bb0828d (#343, 2024-09-25) and
// unchanged since. Copyright 2020-2025 Buf Technologies, Inc., licensed under
// the Apache License 2.0 (the repository's own license). It is stored
// gzipped, 4 KB.
//
// Releases before bufbuild/protocompile#343 (a lexer fix; v0.14.1 is one) take
// about 3.2-3.9s to compile it, and hit the 5s compile timeout under -race.
// go.mod pins a commit that has the fix for that reason.
//
// A compile timeout is not cached, so a schema that hits it costs the CPU
// again on every record: this input must compile (to an error, it is not a
// schema) well inside the compile timeout. The error check catches the
// timeout under -race. The wall-clock bound runs only without -race, where
// the fixed lexer takes about 20ms and the unfixed one 3s or more; under
// -race the fixed lexer's time on a loaded CI runner varies too much for a
// fixed bound.
func TestParse_LexerPathologicalInputCompilesFast(t *testing.T) {
	f, err := os.Open("testdata/pathological3.proto.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	text, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if len(text) != 669047 || bytes.Count(text, []byte{';'}) < 100000 {
		t.Fatalf("testdata is not pathological3.proto (len %d)", len(text))
	}

	start := time.Now()
	_, err = Parse(t.Context(), text)
	elapsed := time.Since(start)

	if !errors.Is(err, ErrSchemaCompile) {
		t.Fatalf("err = %v, want ErrSchemaCompile (the input is not a valid schema)", err)
	}
	if !raceEnabled && elapsed > 2500*time.Millisecond {
		t.Fatalf("compile took %s; the lexer fix (protocompile#343) is missing from the pinned version", elapsed)
	}
}
