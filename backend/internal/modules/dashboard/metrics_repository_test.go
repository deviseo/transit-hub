package dashboard

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type capturingMetricsDB struct {
	query     string
	queryArgs []any
	queryErr  error
}

func (f *capturingMetricsDB) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (f *capturingMetricsDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	f.query = sql
	f.queryArgs = args
	return nil, f.queryErr
}

func (f *capturingMetricsDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return nil
}

func TestListRangeUsesExplicitBusinessDateArgument(t *testing.T) {
	queryErr := errors.New("stop after capturing query")
	db := &capturingMetricsDB{queryErr: queryErr}
	repo := newMetricsRepository(db)

	_, err := repo.ListRange(context.Background(), "user-1", "account-1", 7, "2026-08-01")
	if !errors.Is(err, queryErr) {
		t.Fatalf("ListRange() error = %v, want capture sentinel", err)
	}
	if strings.Contains(db.query, "CURRENT_DATE") {
		t.Fatalf("ListRange query still depends on database CURRENT_DATE: %s", db.query)
	}
	wantArgs := []any{"user-1", "account-1", "2026-08-01", 7}
	if !reflect.DeepEqual(db.queryArgs, wantArgs) {
		t.Fatalf("ListRange query args = %#v, want %#v", db.queryArgs, wantArgs)
	}
}
