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

package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
)

type Txn struct {
	ctx    context.Context //nolint:containedctx // This is a transaction context
	logger zerolog.Logger
	tx     pgx.Tx
}

func (t *Txn) Commit() error {
	if err := t.tx.Commit(t.ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

func (t *Txn) Discard() {
	err := t.tx.Rollback(t.ctx)
	if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		t.logger.Err(err).Ctx(t.ctx).Msg("could not discard transaction")
	}
}
