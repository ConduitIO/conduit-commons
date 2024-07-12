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
	"strings"

	"github.com/conduitio/conduit-commons/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/tracelog"
	"github.com/rs/zerolog"
)

// DB implements the database.DB interface by storing data in Postgres. All the
// data is stored in a single table that acts as a key/value store.
type DB struct {
	logger zerolog.Logger
	pool   *pgxpool.Pool
	table  string
}

var _ database.DB = (*DB)(nil)

// New connects to Postgres and creates the target table if it does not exist.
// It returns a *DB with a pool of connections used for future interactions with
// Postgres. Close needs to be called before exiting to close any open
// connections.
//
// Refer to pgxpool.ParseConfig for details about the connection string format.
//
//	# Example DSN
//	user=jack password=secret host=pg.example.com port=5432 dbname=mydb sslmode=verify-ca pool_max_conns=10
//
//	# Example URL
//	postgres://jack:secret@pg.example.com:5432/mydb?sslmode=verify-ca&pool_max_conns=10
func New(
	ctx context.Context,
	l zerolog.Logger,
	connString string,
	table string,
) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("could not parse connection string: %w", err)
	}

	if strings.TrimSpace(table) == "" {
		return nil, errors.New("postgres DB requires a table")
	}

	l = l.With().Str("component", "postgres.DB").Logger()
	cfg.ConnConfig.Tracer = &tracelog.TraceLog{
		Logger:   (*logger)(&l),
		LogLevel: tracelog.LogLevelTrace, // we control the log level with our own logger
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("could not connect to postgres: %w", err)
	}

	db := &DB{
		logger: l,
		pool:   pool,
		table:  table,
	}
	err = db.init(ctx)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("could not initialize postgres: %w", err)
	}

	return db, nil
}

// Init initializes the database structures needed by DB.
func (d *DB) init(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %q (
			key   VARCHAR PRIMARY KEY,
			value BYTEA
		)`, d.table)
	_, err := d.pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("could not create table %q: %w", d.table, err)
	}
	return nil
}

func (d *DB) Ping(ctx context.Context) error {
	if err := d.pool.Ping(ctx); err != nil {
		return fmt.Errorf("could not ping postgres database: %w", err)
	}
	return nil
}

// NewTransaction starts a new transaction. The `update` parameter controls the
// access mode ("read only" or "read write"). It returns the transaction as well
// as a context that contains the transaction. Passing the context in future
// calls to *DB will execute that action in that transaction.
func (d *DB) NewTransaction(ctx context.Context, update bool) (database.Transaction, context.Context, error) {
	accessMode := pgx.ReadOnly
	if update {
		accessMode = pgx.ReadWrite
	}

	pgxTx, err := d.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: accessMode})
	if err != nil {
		return nil, ctx, fmt.Errorf("could not begin transaction: %w", err)
	}

	txn := &Txn{
		ctx:    ctx,
		logger: d.logger,
		tx:     pgxTx,
	}
	return txn, database.ContextWithTransaction(ctx, txn), nil
}

// Close closes all open connections.
func (d *DB) Close() error {
	if d.pool != nil {
		d.pool.Close()
	}
	return nil
}

// Set will store the value under the key. If value is `nil` we consider that a
// delete.
func (d *DB) Set(ctx context.Context, key string, value []byte) error {
	switch value {
	case nil:
		return d.delete(ctx, key)
	default:
		return d.upsert(ctx, key, value)
	}
}

func (d *DB) delete(ctx context.Context, key string) error {
	query := fmt.Sprintf(`DELETE FROM %q WHERE key = $1`, d.table)
	_, err := d.getQuerier(ctx).Exec(ctx, query, key)
	if err != nil {
		return fmt.Errorf("could not delete key %q: %w", key, err)
	}
	return nil
}

func (d *DB) upsert(ctx context.Context, key string, value []byte) error {
	query := fmt.Sprintf(`
		INSERT INTO %q (key, value)
		VALUES ($1, $2)
		ON CONFLICT (key)
		DO UPDATE SET value = EXCLUDED.value`, d.table)
	_, err := d.getQuerier(ctx).Exec(ctx, query, key, value)
	if err != nil {
		return fmt.Errorf("could not upsert key %q: %w", key, err)
	}
	return nil
}

// Get returns the value stored under the key. If no value is found it returns
// database.ErrKeyNotExist.
func (d *DB) Get(ctx context.Context, key string) ([]byte, error) {
	query := fmt.Sprintf(`SELECT value FROM %q WHERE key = $1`, d.table)
	row := d.getQuerier(ctx).QueryRow(ctx, query, key)

	var value []byte
	err := row.Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		// translate error
		err = database.ErrKeyNotExist
	}
	if err != nil {
		return nil, fmt.Errorf("could not select key %q: %w", key, err)
	}
	return value, nil
}

// GetKeys returns all keys with the requested prefix. If prefix is an empty
// string it will return all keys.
func (d *DB) GetKeys(ctx context.Context, prefix string) ([]string, error) {
	query := fmt.Sprintf(`SELECT key FROM %q`, d.table)
	var args []interface{}
	if prefix != "" {
		query += " WHERE key LIKE $1"
		args = append(args, prefix+"%")
	}
	rows, err := d.getQuerier(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("could not select keys with prefix %q: %w", prefix, err)
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var value string
		err := rows.Scan(&value)
		if err != nil {
			return nil, fmt.Errorf("could not scan value: %w", err)
		}
		values = append(values, value)
	}

	return values, nil
}

// getQuerier tries to take the transaction out of the context, if it does not
// find a transaction it falls back directly to the postgres connection.
func (d *DB) getQuerier(ctx context.Context) querier {
	txn := d.getTxn(ctx)
	if txn != nil {
		return txn.tx
	}
	return d.pool
}

// getTxn takes the transaction out of the context and returns it. If the
// context does not contain a transaction it returns nil.
func (d *DB) getTxn(ctx context.Context) *Txn {
	txn, ok := database.TransactionFromContext(ctx).(*Txn)
	if !ok {
		return nil
	}
	return txn
}

type querier interface {
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, optionsAndArgs ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, optionsAndArgs ...interface{}) pgx.Row
}
