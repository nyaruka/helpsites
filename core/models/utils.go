package models

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/nyaruka/gocommon/dbutil"
)

// DBorTx is the interface for a database or transaction
type DBorTx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// ScanJSONRows scans rows of JSON into values created by the given function
func ScanJSONRows[T any](rows *sql.Rows, f func() T) ([]T, error) {
	defer rows.Close()

	as := make([]T, 0, 10)
	for rows.Next() {
		a := f()
		if err := dbutil.ScanJSON(rows, &a); err != nil {
			return nil, fmt.Errorf("error scanning into %T: %w", a, err)
		}
		as = append(as, a)
	}

	return as, nil
}

// queryJSON runs the given query and scans its rows of JSON into values created by the given function
func queryJSON[T any](ctx context.Context, db DBorTx, f func() T, query string, args ...any) ([]T, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return ScanJSONRows(rows, f)
}

// queryJSONOne runs the given query and scans its single row of JSON into a value created by the given function,
// returning nil if there's no such row
func queryJSONOne[T any](ctx context.Context, db DBorTx, f func() T, query string, args ...any) (T, error) {
	var zero T

	rows, err := queryJSON(ctx, db, f, query, args...)
	if err != nil {
		return zero, err
	}
	if len(rows) == 0 {
		return zero, nil
	}
	return rows[0], nil
}
