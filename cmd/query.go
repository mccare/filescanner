package cmd

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var Directory string
var Duplicate bool

func query() {
	db := DBConnect()
	defer db.Close(context.Background())

	file, err := os.Open("query.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	query, err := reader.ReadString('\n')

	fmt.Printf("Running Query %s\n", query)

	rows, err := db.Query(context.Background(), query)
	if err != nil {
		fmt.Println("error during query", err)
		os.Exit(1)
	}
	for rows.Next() {
		var path string
		rows.Scan(&path)
		fmt.Println(path)
	}
}

func findDuplicates() {
	db := DBConnect()
	defer db.Close(context.Background())
	rows, err := db.Query(context.Background(),
		"select path, md5 from music_files where path like $1 and md5 in (select md5 from music_files where path like $2 group by md5 having count(id) > 1) order by md5", Directory+"%", Directory+"%")
	if err != nil {
		fmt.Println("Error during query", err)
	}
	var md5s = make(map[uuid.UUID]string)

	for rows.Next() {
		var path string
		var md5 uuid.UUID

		rows.Scan(&path, &md5)
		if md5s[md5] != "" {
			fmt.Println(path)
		}
		md5s[md5] = path
	}
}

func NewQueryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "query",
		Short: "query the DB",
		Run: func(cmd *cobra.Command, args []string) {
			if Duplicate {
				findDuplicates()
				return
			}
			query()
		},
	}
	cmd.Flags().StringVarP(&Directory, "path", "p", "", "Path directory to query")
	cmd.Flags().BoolVarP(&Duplicate, "duplicate", "d", false, "Find duplicates within one directory, just keep the first one and output all others")
	cmd.MarkFlagRequired("path")
	return cmd
}
