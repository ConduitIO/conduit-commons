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

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/conduitio/conduit-commons/database"
	"github.com/rs/zerolog"
)

type Transaction struct {
	ctx    context.Context //nolint:containedctx // This is a transaction context
	tx     *sql.Tx
	logger zerolog.Logger
}

var _ database.Transaction = (*Transaction)(nil)

func (t *Transaction) Commit() error {
	if err := t.tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

func (t *Transaction) Discard() {
	if err := t.tx.Rollback(); err != nil {
		if !errors.Is(err, sql.ErrTxDone) {
			t.logger.Err(err).Ctx(t.ctx).Msg("could not discard tx")
		}
	}
}
