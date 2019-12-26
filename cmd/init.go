package cmd

import (
	"context"

	"github.com/spf13/cobra"
)

func createDatabase() {
	conn := DBConnect()
	defer conn.Close(context.Background())
	//	conn.Exec(context.Background(), "drop table files")
	conn.Exec(context.Background(), `create table files ( 
		id UUID,
		path VARCHAR UNIQUE,
		size INT,
		md5 UUID,
		deleted bool default false,
		extension varchar
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
