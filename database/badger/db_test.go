// Copyright © 2022 Meroxa, Inc.
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

package badger

import (
	"path/filepath"
	"testing"

	"github.com/conduitio/conduit-commons/database"
	"github.com/matryer/is"
	"github.com/rs/zerolog"
)

func TestDB(t *testing.T) {
	is := is.New(t)

	path := filepath.Join(t.TempDir(), "badger.db")
	badger, err := New(zerolog.Nop(), path)
	is.NoErr(err)
	t.Cleanup(func() {
		err := badger.Close()
		is.NoErr(err)
	})
	database.AcceptanceTest(t, badger)
}
