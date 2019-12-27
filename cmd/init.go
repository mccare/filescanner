package cmd

import (
	"context"

	"github.com/spf13/cobra"
)

func createDatabase() {
	conn := DBConnect()
	defer conn.Close(context.Background())
	//	conn.Exec(context.Background(), "drop view music_files")
	//	conn.Exec(context.Background(), "drop table files")
	conn.Exec(context.Background(), `create table files ( 
		id UUID,
		path VARCHAR UNIQUE,
		size INT,
		md5 UUID,
		deleted bool default false,
		extension varchar,
		id3_album varchar,
		id3_album_artist varchar,
		id3_title varchar,
		id3_artist varchar,
		id3_composer varchar,
		id3_scanned bool default false
	);`)
	conn.Exec(context.Background(), "create index on files(size)")
	conn.Exec(context.Background(), `create or replace view music_files as select * from files where extension in ( 'm4b', 'm4p', 'm4a', 'mp3', 'ogg') and not deleted`)
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
