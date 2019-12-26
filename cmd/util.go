package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v4"
)

func DBConnect() *pgx.Conn {
	conn, err := pgx.Connect(context.Background(), "postgresql://chris:cvdl@localhost/filescanner")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connection to database: %v\n", err)
		os.Exit(1)
	}
	return conn
}
