package cmd

import (
	"context"
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	pb "github.com/cheggaaa/pb/v3"
	"github.com/dhowden/tag"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/spf13/cobra"
)

type File struct {
	Path        string
	Size        int64
	ID          uuid.UUID
	Md5         [md5.Size]byte
	ID3Scanned  bool
	Extension   string
	Filename    string
	ID3Md5      string
	Deleted     bool
	Artist      string
	AlbumArtist string
	Title       string
	Album       string
}

var Path string
var Checkdb bool
var ScanTags bool
var MaxDBConnections = 10
var MaxFileConnections = make(chan int, 80)

func (f *File) hasMd5() bool {
	for i := 0; i < md5.Size; i++ {
		if f.Md5[i] != 0 {
			return true
		}
	}
	return false
}

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

func insertFile(dbs chan *pgx.Conn, f File) error {
	db := <-dbs

	defer func() {
		dbs <- db
	}()

	_, err := db.Exec(context.Background(), `insert into files (id, filename, extension, path, id3_artist, id3_album_artist, id3_album, id3_title, id3_md5, id3_scanned, deleted, size, md5)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		f.ID, f.Filename, f.Extension, f.Path, f.Artist, f.AlbumArtist, f.Album, f.Title, f.ID3Md5, f.ID3Scanned, f.Deleted, f.Size, f.Md5)
	if err != nil {
		fmt.Println("Error during insert", err)
		return err
	}
	return nil
}

func queryFiles(dbs chan *pgx.Conn, tablename string, where string, args ...interface{}) ([]File, error) {
	db := <-dbs

	defer func() {
		dbs <- db
	}()

	rows, err := (*db).Query(context.Background(), `select id, filename, extension, path, id3_artist, id3_album_artist, id3_album, id3_title, id3_md5, id3_scanned, deleted, size, md5
		from `+tablename+` where `+where, args...)
	if err != nil {
		fmt.Println("Error during query", err)
		return nil, err
	}

	var result []File
	for rows.Next() {
		var f File
		rows.Scan(&f.ID, &f.Filename, &f.Extension, &f.Path, &f.Artist, &f.AlbumArtist, &f.Album, &f.Title, &f.ID3Md5, &f.ID3Scanned, &f.Deleted, &f.Size, &f.Md5)
		result = append(result, f)
		if rows.Err() != nil {
			fmt.Println("Error during Scan", rows.Err())
			return nil, rows.Err()
		}
	}
	rows.Close()

	return result, nil
}

func updateExtensionAndFilename(file *File) {
	extensionRe := regexp.MustCompile(`\.(\w+)$`)
	filenameRe := regexp.MustCompile(`/([^/]+)$`)

	for match := filenameRe.FindStringSubmatch(file.Path); match != nil; {
		file.Filename = strings.ToLower(match[1])
		break
	}
	for match := extensionRe.FindStringSubmatch(file.Path); match != nil; {
		file.Extension = strings.ToLower(match[1])
		break
	}
}

// insert the file inot the DB (if not already present) and return the File
func insertFiles(done <-chan struct{}, input <-chan File, output chan<- File) {
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
				updateExtensionAndFilename(&file)
				output <- file
			}
			return nil
		})
	}()

	wg.Add(numFileInserter)
	fileInserterWaiter.Add(numFileInserter)
	for i := 0; i < numFileInserter; i++ {
		go func() {
			insertFiles(done, output, md5Files)
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

func scanForDelete(dbs chan *pgx.Conn, file File) {
	if _, err := os.Stat(file.Path); os.IsNotExist(err) {
		fmt.Println("Does not exist", file.Path)
		db := <-dbs
		_, err := db.Exec(context.Background(), "update files set deleted = true where id = $1", file.ID)
		dbs <- db
		if err != nil {
			fmt.Println("Error during update", err)
			os.Exit(1)
		}
	}
}

func scanDB(processor func(dbs chan *pgx.Conn, file File), tableName string, processors int) {
	var wg sync.WaitGroup

	dbs := make(chan *pgx.Conn, MaxDBConnections)
	for i := 0; i < MaxDBConnections; i++ {
		dbs <- DBConnect()
	}

	defer func() {
		for i := 0; i < MaxDBConnections; i++ {
			db := <-dbs
			db.Close(context.Background())
		}
	}()
	files, _ := queryFiles(dbs, tableName, `path like $1 and not deleted`, Path+`%`)
	fmt.Println("Processing files: ", len(files))
	bar := pb.StartNew(len(files))

	fileQueue := make(chan File, 1)

	for _, fileToProcess := range files {
		fileQueue <- fileToProcess
		wg.Add(1)
		go func() {
			processor(dbs, <-fileQueue)
			bar.Increment()
			wg.Done()
		}()
	}

	wg.Wait()
}

func scanTags(dbs chan *pgx.Conn, file File) {
	if file.ID3Scanned {
		return
	}

	func() {
		MaxFileConnections <- 1
		f, err := os.Open(file.Path)
		if err != nil {
			fmt.Printf("error loading file: %v", err)
			return
		}
		defer func() {
			f.Close()
			<-MaxFileConnections
		}()

		var sum string
		m, err := tag.ReadFrom(f)
		if err != nil {
			fmt.Printf("error reading file: %v\n", err)
		} else {
			sum, err = tag.Sum(f)
			if err != nil {
				fmt.Printf("error calculating sum on %v: %v\n", file.Path, err)
				sum = ""
			}
		}

		db := <-dbs
		_, err = db.Exec(context.Background(),
			`update files 
				set id3_album = $1, id3_album_artist = $2, id3_artist = $3, id3_title = $4, id3_composer = $5, id3_scanned = true, id3_md5 = $6
				where id = $7`,
			m.Album(), m.AlbumArtist(), m.Artist(), m.Title(), m.Composer(), sum, file.ID)
		dbs <- db
		if err != nil {
			fmt.Printf("Error during update on %v error %v", file.Path, err)
		}
	}()
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
			// scanFolder()
		},
	}
	cmd.Flags().StringVarP(&Path, "path", "p", "", "Path directory to scan")
	cmd.Flags().BoolVarP(&Checkdb, "checkdb", "c", false, "scan the database and see if the files still exist")
	cmd.Flags().BoolVarP(&ScanTags, "scantags", "s", false, "scan the files for ID3 tags")
	cmd.MarkFlagRequired("path")
	return cmd
}
