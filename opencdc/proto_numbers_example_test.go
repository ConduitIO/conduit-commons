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

package opencdc_test

import (
	"errors"
	"fmt"

	"github.com/conduitio/conduit-commons/opencdc"
)

// A record whose key does not fit in a protobuf number is refused, with the
// field path, instead of being rounded on its way to a standalone plugin.
func ExampleRecord_ValidateProto() {
	rec := opencdc.Record{
		Key: opencdc.StructuredData{"id": int64(9007199254740993)}, // 2^53+1
	}

	err := rec.ValidateProto()
	fmt.Println(errors.Is(err, opencdc.ErrIntegerNotRepresentable))

	var e *opencdc.IntegerNotRepresentableError
	if errors.As(err, &e) {
		fmt.Println(e.Path, e.GoType)
	}

	// Output:
	// true
	// key.id int64
}
