package cmd

import (
	"context"
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	pb "github.com/cheggaaa/pb/v3"
	"github.com/dhowden/tag"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

type File struct {
	Path string
	Size int64
	Id   string
	Md5  [md5.Size]byte
}

var Path string
var Checkdb bool
var ScanTags bool

func (f *File) hasMd5() bool {
	for i := 0; i < md5.Size; i++ {
		if f.Md5[i] != 0 {
			return true
		}
	}
	return false
}

// Go Routine structure
//   scanFolder -> send paths/info Fan out -> insertFile (check if existing, insert, check for duplicate, if yes) -> FileMD5Writer

func conditionalMd5Writer(done <-chan struct{}, input <-chan File, output chan<- File) {
	db := DBConnect()
	defer db.Close(context.Background())
	for file := range input {
		var existing int64
		db.QueryRow(context.Background(), `select count(*) from files where size = $1`, file.Size).Scan(&existing)
		if existing > 1 {
			if !file.hasMd5() {
				// need to calculated MD5 if md5 is null
				fmt.Printf("MD5 %s\n", file.Path)
				data, err := ioutil.ReadFile(file.Path)
				if err != nil {
					fmt.Printf("ERROR during reading of file %s", file.Path)
				} else {
					file.Md5 = md5.Sum(data)
					db.Exec(context.Background(), `update files set md5 = $1 where path = $2`, file.Md5, file.Path)
				}
			}
		}
		select {
		case output <- file:
		case <-done:
			fmt.Printf("Closing signal on done queue")
			return
		}
	}
}

// insert the file inot the DB (if not already present) and return the File
func insertFile(done <-chan struct{}, input <-chan File, output chan<- File) {
	db := DBConnect()
	defer db.Close(context.Background())
	for file := range input {
		var existing int64
		db.QueryRow(context.Background(), `select count(*) from files where path = $1`, file.Path).Scan(&existing)
		if existing == 0 {
			var newId uuid.UUID
			db.QueryRow(context.Background(), `insert into files(id, path, size) values ($1, $2, $3) returning (id)`, uuid.New(), file.Path, file.Size).Scan(&newId)
			fmt.Printf("New %s with %s\n", file.Path, newId)
		} else {
			var md5 [md5.Size]byte
			db.QueryRow(context.Background(), `select md5 from files where path = $1`, file.Path).Scan(&md5)
			file.Md5 = md5
		}
		select {
		case output <- file:
		case <-done:
			return
		}
	}
}

// scan folder is synchronous, will return after all workers have finished
func scanFolder() {
	// just an estimate for the progress bar
	count := 250000
	bar := pb.StartNew(count)
	output := make(chan File)
	md5Files := make(chan File, 20000)
	insertedFile := make(chan File)
	done := make(chan struct{})
	numFileInserter := 10
	var wg sync.WaitGroup
	var fileInserterWaiter sync.WaitGroup

	// security measure, if the routine fails with error/panic

	go func() {
		defer close(output)
		filepath.Walk(Path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				bar.Increment()
				output <- File{Path: path, Size: info.Size()}
			}
			return nil
		})
	}()

	wg.Add(numFileInserter)
	fileInserterWaiter.Add(numFileInserter)
	for i := 0; i < numFileInserter; i++ {
		go func() {
			insertFile(done, output, md5Files)
			wg.Done()
			fileInserterWaiter.Done()
		}()
	}

	go func() {
		fileInserterWaiter.Wait()
		close(md5Files)
	}()

	wg.Add(numFileInserter)
	for i := 0; i < numFileInserter; i++ {
		go func() {
			conditionalMd5Writer(done, md5Files, insertedFile)
			wg.Done()
		}()
	}

	go func() {
		for _ = range insertedFile {
		}
		//			fmt.Printf("Processed file %s", c.Path)
		//		}
	}()

	wg.Wait()
}

func deleteFile(input <-chan uuid.UUID) {
	db := DBConnect()
	defer db.Close(context.Background())
	for id := range input {
		_, err := db.Exec(context.Background(), "update files set deleted = true where id = $1", id)
		fmt.Println("Deleting", id)
		if err != nil {
			fmt.Println("Error during update", err)
			os.Exit(1)
		}
	}
}

func scanDB() {
	var wg sync.WaitGroup

	db := DBConnect()
	defer db.Close(context.Background())

	var rowCount int
	filesToDelete := make(chan uuid.UUID)

	wg.Add(1)
	go func() {
		deleteFile(filesToDelete)
		wg.Done()
	}()

	db.QueryRow(context.Background(), `select count(*) from files where path like $1 and not deleted `, Path+`%`).Scan(&rowCount)

	bar := pb.StartNew(rowCount)

	rows, err := db.Query(context.Background(), "select path, id from files where path like $1 and not deleted", Path+"%")
	if err != nil {
		fmt.Println("Error during query")
		os.Exit(1)
	}
	for rows.Next() {
		bar.Increment()
		var path string
		var id uuid.UUID
		rows.Scan(&path, &id)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			fmt.Printf("Does not exist %s\n", path)
			filesToDelete <- id
		}
	}
	close(filesToDelete)
	wg.Wait()
}

func scanTags() {
	db := DBConnect()
	defer db.Close(context.Background())

	var rowCount int

	db.QueryRow(context.Background(), `select count(*) from music_files where path like $1`, Path+`%`).Scan(&rowCount)
	bar := pb.StartNew(rowCount)
	rows, err := db.Query(context.Background(), "select path, id from music_files where path like $1", Path+"%")
	if err != nil {
		fmt.Println("Error during query")
		os.Exit(1)
	}
	for rows.Next() {
		bar.Increment()
		var path string
		var id uuid.UUID
		rows.Scan(&path, &id)
		f, err := os.Open(path)
		if err != nil {
			fmt.Printf("error loading file: %v", err)
			continue
		}
		m, err := tag.ReadFrom(f)
		if err != nil {
			fmt.Printf("error reading file: %v\n", err)
			continue
		}
		fmt.Printf("   Album:  %v\n", m.Album())
		fmt.Printf("   Artist: %v\n", m.Artist())
		fmt.Printf("   Title:  %v\n", m.Title())
	}

}

func NewScanCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "scan a folder",
		Run: func(cmd *cobra.Command, args []string) {
			if Checkdb {
				scanDB()
				return
			}
			if ScanTags {
				scanTags()
				return
			}
			scanFolder()
		},
	}
	cmd.Flags().StringVarP(&Path, "path", "p", "", "Path directory to scan")
	cmd.Flags().BoolVarP(&Checkdb, "checkdb", "c", false, "scan the database and see if the files still exist")
	cmd.Flags().BoolVarP(&ScanTags, "scantags", "s", false, "scan the files for ID3 tags")
	cmd.MarkFlagRequired("path")
	return cmd
}
