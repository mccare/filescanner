package cmd

import (
	"context"
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	pb "github.com/cheggaaa/pb/v3"
	"github.com/dhowden/tag"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

type token struct{}

type Processor func(file File) File

// Arguments that are parsed via cobra
var Path string
var Checkdb bool
var ScanTags bool

// Processor calculates and updates MD5 if not already done
func updateMd5(file File) File {
	if !file.hasMd5() {
		// need to calculated MD5 if md5 is null
		fmt.Println("MD5 calcuation for ", file.Path, file.Md5)
		data, err := ioutil.ReadFile(file.Path)
		if err != nil {
			fmt.Printf("ERROR during reading of file %s", file.Path)
		} else {
			file.Md5 = md5.Sum(data)
			db := DBConnectionPool.Get()
			defer func() {
				DBConnectionPool.Release(db)
			}()
			db.Exec(context.Background(), `update files set md5 = $1 where id = $2`, file.Md5, file.ID)
		}
	}
	return file
}

// insert the file into the DB (if not already present) and return the File
func insertFileIfNew(file File) File {
	if file.ID == uuid.Nil {
		fmt.Println("New file ", file.Path)
		file.updateExtensionAndFilename()
		file.ID = uuid.New()
		InsertFile(file)
	}
	return file
}

// scan folder is synchronous, will return after all workers have finished
func scanFolder(maxProcessors int, processors ...Processor) {
	// just an estimate for the progress bar
	count := 50000
	bar := pb.StartNew(count)
	sem := make(chan token, maxProcessors)
	filepath.Walk(Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Println("Error during file walk", err)
			return err
		}
		if info.Mode().IsRegular() {
			sem <- token{}
			bar.Increment()

			go func(path string, info os.FileInfo) {
				file := QueryFileByPath(path)
				file.Size = info.Size()
				for _, p := range processors {
					file = p(file)
				}
				<-sem
			}(path, info)
		}
		return nil
	})

	// waiting for the  children to finish
	for i := 0; i < maxProcessors; i++ {
		sem <- token{}
	}
}

// Processor to see if file exists, if not will update DB
func checkAndMarkDeletedFile(file File) File {
	if _, err := os.Stat(file.Path); os.IsNotExist(err) {
		fmt.Println("Does not exist", file.Path)
		db := DBConnectionPool.Get()
		_, err := db.Exec(context.Background(), "update files set deleted = true where id = $1", file.ID)
		DBConnectionPool.Release(db)
		if err != nil {
			fmt.Println("Error during update", err)
			os.Exit(1)

		}
	}
	return file
}

// Processor to read ID3 Tags and calculated MD5 from sound file only. Also updates DB
// TODO information retrieved is not copied into the returned file object!
func scanTagsAndUpdateDB(file File) File {
	if file.ID3Scanned {
		return file
	}
	if !file.MusicFile() {
		return file
	}
	fmt.Println("ID3 MD5 for ", file.Path)
	f, err := os.Open(file.Path)
	if err != nil {
		fmt.Printf("error loading file: %v", err)
		return file
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		fmt.Printf("error reading tags: %v\n", err)
	} else {
		db := DBConnectionPool.Get()
		_, err = db.Exec(context.Background(),
			`update files 
				set id3_album = $1, id3_album_artist = $2, id3_artist = $3, id3_title = $4, id3_composer = $5, id3_scanned = true
				where id = $6`,
			m.Album(), m.AlbumArtist(), m.Artist(), m.Title(), m.Composer(), file.ID)
		if err != nil {
			fmt.Printf("Error during update on %v error %v", file.Path, err)
		}
		DBConnectionPool.Release(db)
	}

	sum, err := tag.Sum(f)
	if err != nil {
		fmt.Printf("error calculating sum on %v: %v\n", file.Path, err)
	} else {
		db := DBConnectionPool.Get()
		_, err = db.Exec(context.Background(),
			`update files 
				set id3_scanned = true, id3_md5 = $1
				where id = $2`,
			sum, file.ID)
		if err != nil {
			fmt.Printf("Error during update on %v error %v", file.Path, err)
		}
		DBConnectionPool.Release(db)
	}
	return file
}

// Wrapper Routine to call different processors on each file, driven by contents of DB
func scanDB(tableName string, numProcessors int, processors ...Processor) {
	files, _ := QueryFiles(tableName, `path like $1 and not deleted`, Path+`%`)
	fmt.Println("Processing files: ", len(files))
	bar := pb.StartNew(len(files))

	maxConcurrent := make(chan token, numProcessors)

	for _, fileToProcess := range files {
		maxConcurrent <- token{}
		go func(file File) {
			for _, p := range processors {
				file = p(file)
			}
			bar.Increment()
			<-maxConcurrent
		}(fileToProcess)
	}
	for i := 0; i < numProcessors; i++ {
		maxConcurrent <- token{}
	}
}

func NewScanCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "scan a folder",
		Run: func(cmd *cobra.Command, args []string) {
			InitDBConnectionPool()

			if Checkdb {
				scanDB("files", 100, checkAndMarkDeletedFile)
				return
			}
			if ScanTags {
				fmt.Printf("Scnaning tags")
				scanDB("music_files", 10, scanTagsAndUpdateDB)
				return
			}
			scanFolder(100, insertFileIfNew, updateMd5, scanTagsAndUpdateDB)
		},
	}
	cmd.Flags().StringVarP(&Path, "path", "p", "", "Path directory to scan")
	cmd.Flags().BoolVarP(&Checkdb, "checkdb", "c", false, "scan the database and see if the files still exist")
	cmd.Flags().BoolVarP(&ScanTags, "scantags", "s", false, "scan the files for ID3 tags")
	cmd.MarkFlagRequired("path")
	return cmd
}
