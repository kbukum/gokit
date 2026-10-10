package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/stdlib"
)

func configureConnection(location *time.Location) stdlib.OptionOpenDB {
	return stdlib.OptionAfterConnect(func(ctx context.Context, conn *pgx.Conn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		conn.TypeMap().RegisterType(&pgtype.Type{
			Name: "timestamp", OID: pgtype.TimestampOID,
			Codec: &pgtype.TimestampCodec{ScanLocation: location},
		})
		return nil
	})
}
