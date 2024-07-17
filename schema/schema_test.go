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

package schema

import (
	"fmt"
	"testing"

	"github.com/matryer/is"
)

func TestType(t *testing.T) {
	testCases := []struct {
		typ  Type
		text []byte
	}{
		{typ: TypeAvro, text: []byte("avro")},
		{typ: Type(2), text: []byte("Type(2)")},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("MarshalText_%s", tc.typ.String()), func(t *testing.T) {
			is := is.New(t)
			text, err := tc.typ.MarshalText()
			is.NoErr(err)
			is.Equal(text, tc.text)
		})
		t.Run(fmt.Sprintf("UnmarshalText_%s", tc.typ.String()), func(t *testing.T) {
			is := is.New(t)
			var typ Type
			err := typ.UnmarshalText(tc.text)
			is.NoErr(err)
			is.Equal(typ, tc.typ)
		})
	}
}
