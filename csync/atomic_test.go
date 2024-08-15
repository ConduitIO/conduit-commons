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

package csync

import (
	"github.com/matryer/is"
	"testing"
)

func TestNewAtomicPointer(t *testing.T) {
	type testCase struct {
		name string
		val  any
	}

	testCases := []testCase{
		{
			name: "string",
			val:  "something",
		},
		{
			name: "nil",
			val:  nil,
		},
		{
			name: "integer",
			val:  123,
		},
		{
			name: "struct",
			val: testCase{
				name: "test test case",
				val:  []string{"a", "b", "c"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)

			ptr := NewAtomicPointer(tc.val)
			got := *ptr.Load()
			is.Equal(got, tc.val)
		})
	}
}