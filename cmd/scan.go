package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/spf13/cobra"
)

type File struct {
	Path string
	Size int64
	Id   string
}

// Go Routine structure
//   scanFolder -> send paths/info Fan out -> insertFile (check if existing, insert, check for duplicate, if yes) -> FileMD5Writer

func connect() *pgx.Conn {
	conn, err := pgx.Connect(context.Background(), "postgresql://chris:cvdl@localhost/filescanner")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connection to database: %v\n", err)
		os.Exit(1)
	}
	return conn
}

// insert the file inot the DB (if not already present) and return the File
func insertFile(done <-chan struct{}, input <-chan File, output chan<- File) {
	db := connect()
	defer db.Close(context.Background())
	for file := range input {
		var existing int64
		db.QueryRow(context.Background(), `select count(*) from files where path = $1`, file.Path).Scan(&existing)
		if existing == 0 {
			var newId uuid.UUID
			db.QueryRow(context.Background(), `insert into files(id, path, size) values ($1, $2, $3) returning (id)`, uuid.New(), file.Path, file.Size).Scan(&newId)
			fmt.Printf("Inserting File %s with %s", file.Path, newId)
			select {
			case output <- file:
			case <-done:
				return
			}
		}
	}
}

// scan folder is synchronous, will return after all workers have finished
func scanFolder() {
	path := `/Users/chris/Music`
	output := make(chan File)
	insertedFile := make(chan File)
	done := make(chan struct{})
	numFileInserter := 5
	var wg sync.WaitGroup

	// security measure, if the routine fails with error/panic

	go func() {
		defer close(output)
		filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				output <- File{path, info.Size(), ""}
			}
			return nil
		})
	}()

	wg.Add(numFileInserter)
	for i := 0; i < numFileInserter; i++ {
		go func() {
			insertFile(done, output, insertedFile)
			wg.Done()
		}()
	}

	go func() {
		for c := range insertedFile {
			fmt.Printf("Processed file %s", c.Path)
		}
	}()

	wg.Wait()
}

func NewScanCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "scan a folder",
		Run: func(cmd *cobra.Command, args []string) {
			scanFolder()
		},
	}
	return cmd
}
