package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func configureConnection(_ context.Context, cfg *pgx.ConnConfig) error {
	cfg.RuntimeParams["statement_timeout"] = "30000"
	cfg.RuntimeParams["idle_in_transaction_session_timeout"] = "10000"
	return nil
}
