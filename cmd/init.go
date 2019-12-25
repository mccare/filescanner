package cmd

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/spf13/cobra"
	"os"
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
		md5 VARCHAR
	);`)

	// Example for the insert
	uuid.New()
	// conn.QueryRow(context.Background(), `insert into files(id, path, size) values ($1, $2, $3)`, uuid.New(), `hello`, 10)
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
