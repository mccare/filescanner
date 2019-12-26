package cmd

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
)

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

func NewQueryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "query",
		Short: "query the DB",
		Run: func(cmd *cobra.Command, args []string) {
			query()
		},
	}
	return cmd
}
