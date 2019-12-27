package cmd

import (
	"context"
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	pb "github.com/cheggaaa/pb/v3"
	"github.com/dhowden/tag"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

type File struct {
	Path       string
	Size       int64
	Id         uuid.UUID
	Md5        [md5.Size]byte
	ID3Scanned bool
	Extension  string
	Filename   string
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

func filenameExtension(file *File) {
	extensionRe := regexp.MustCompile(`\.(\w+)$`)
	filenameRe := regexp.MustCompile(`/([^/]+)$`)

	for match := filenameRe.FindStringSubmatch(file.Path); match != nil; {
		file.Filename = match[1]
		break
	}
	for match := extensionRe.FindStringSubmatch(file.Path); match != nil; {
		file.Extension = match[1]
		break
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
			var newID uuid.UUID
			db.QueryRow(context.Background(), `insert into files(id, path, size, filename, extension) 
				values ($1, $2, $3, $4, $5) returning (id)`,
				uuid.New(), file.Path, file.Size, file.Filename, file.Extension).Scan(&newID)
			fmt.Printf("New %s with %s\n", file.Path, newID)
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
				file := File{Path: path, Size: info.Size()}
				filenameExtension(&file)
				output <- file
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

func scanForDelete(input <-chan File) {
	db := DBConnect()
	defer db.Close(context.Background())
	for file := range input {
		if _, err := os.Stat(file.Path); os.IsNotExist(err) {
			fmt.Printf("Does not exist %s\n", file.Path)
			_, err := db.Exec(context.Background(), "update files set deleted = true where id = $1", file.Id)
			if err != nil {
				fmt.Println("Error during update", err)
				os.Exit(1)
			}
		}
	}
}

func scanDB(processor func(<-chan File), tableName string, processors int) {
	var wg sync.WaitGroup

	db := DBConnect()
	defer db.Close(context.Background())

	var rowCount int
	filePipeline := make(chan File)
	for i := 1; i < processors; i++ {
		wg.Add(1)
		go func() {
			processor(filePipeline)
			wg.Done()
		}()
	}

	db.QueryRow(context.Background(), `select count(*) from `+tableName+` where path like $1 and not deleted `, Path+`%`).Scan(&rowCount)

	bar := pb.StartNew(rowCount)

	rows, err := db.Query(context.Background(), `select path, id, id3_scanned, extension, filename from `+tableName+` where path like $1 and not deleted`, Path+"%")
	if err != nil {
		fmt.Println("Error during query")
		os.Exit(1)
	}
	for rows.Next() {
		bar.Increment()
		file := File{}
		rows.Scan(&file.Path, &file.Id, &file.ID3Scanned, &file.Extension, &file.Filename)
		filePipeline <- file
	}
	if rows.Err() != nil {
		fmt.Println("Error during processing ", rows.Err())
	}
	close(filePipeline)
	fmt.Println("Waiting for all my children")
	wg.Wait()
	fmt.Println("Now exiting")
}

func scanTags(input <-chan File) {
	db := DBConnect()
	defer db.Close(context.Background())
	for file := range input {

		if file.ID3Scanned {
			continue
		}

		func() {
			f, err := os.Open(file.Path)
			if err != nil {
				fmt.Printf("error loading file: %v", err)
				return
			}
			defer f.Close()

			m, err := tag.ReadFrom(f)
			if err != nil {
				fmt.Printf("error reading file: %v\n", err)
				return
			}

			_, err = db.Exec(context.Background(),
				`update files 
					set id3_album = $1, id3_album_artist = $2, id3_artist = $3, id3_title = $4, id3_composer = $5, id3_scanned = true
					where id = $6`,
				m.Album(), m.AlbumArtist(), m.Artist(), m.Title(), m.Composer(), file.Id)
			if err != nil {
				fmt.Printf("Error during update on %v error %v", file.Path, err)
			}
		}()
	}
}

func NewScanCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "scan a folder",
		Run: func(cmd *cobra.Command, args []string) {
			if Checkdb {
				scanDB(scanForDelete, "files", 3)
				return
			}
			if ScanTags {
				fmt.Printf("Scnaning tags")
				scanDB(scanTags, "music_files", 6)
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
