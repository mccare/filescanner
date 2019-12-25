package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v4"
	"github.com/spf13/cobra"
)

func createDatabase() {
	conn, err := pgx.Connect(context.Background(), "postgresql://chris:cvdl@localhost/filescanner")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connection to database: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(context.Background())
	conn.Exec(context.Background(), "drop table files")
	conn.Exec(context.Background(), `create table files ( 
		id UUID,
		path VARCHAR UNIQUE,
		size INT,
		md5 UUID
	);`)
	conn.Exec(context.Background(), "create index on files(size)")
}

func NewInitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "initialize database",
		Run: func(cmd *cobra.Command, args []string) {
			createDatabase()
		},
	}
	return cmd
}
