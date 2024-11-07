// Copyright © 2024 Meroxa, Inc.
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

package paramgen

import (
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/matryer/is"
)

func TestIntegration(t *testing.T) {
	testCases := []struct {
		havePath   string
		structName string
		wantPath   string
	}{{
		havePath:   "./testdata/basic",
		structName: "SourceConfig",
		wantPath:   "./testdata/basic/want.go",
	}, {
		havePath:   "./testdata/complex",
		structName: "SourceConfig",
		wantPath:   "./testdata/complex/want.go",
	}, {
		havePath:   "./testdata/tags",
		structName: "Config",
		wantPath:   "./testdata/tags/want.go",
	}, {
		havePath:   "./testdata/dependencies",
		structName: "Config",
		wantPath:   "./testdata/dependencies/want.go",
	}}

	for _, tc := range testCases {
		t.Run(tc.havePath, func(t *testing.T) {
			is := is.New(t)
			params, pkg, err := ParseParameters(tc.havePath, tc.structName)
			is.NoErr(err)
			want, err := os.ReadFile(tc.wantPath)
			is.NoErr(err)
			got := GenerateCode(params, pkg, tc.structName)
			is.Equal("", cmp.Diff(string(want), got))
		})
	}
}
