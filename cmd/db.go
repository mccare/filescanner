package cmd

import (
	"context"
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
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

var FileExtensions = map[string]string{
	`mp3`: `music`,
	`ogg`: `music`,
	`m4a`: `music`,
	`m4b`: `music`,
	`m4p`: `music`,
}

func (f *File) hasMd5() bool {
	for i := 0; i < md5.Size; i++ {
		if f.Md5[i] != 0 {
			return true
		}
	}
	return false
}

func (f *File) MusicFile() bool {
	value, found := FileExtensions[f.Extension]
	return found && value == `music`
}

func (f *File) ArtistAlbumDirectory() []string {
	var result []string
	artist := f.AlbumArtist
	if artist == `` {
		artist = f.Artist
		if artist == `` {
			return result
		}
	}
	if f.Album == `` {
		return result
	}
	result = append(result, artist)
	result = append(result, f.Album)
	return result
}

func (f *File) updateExtensionAndFilename() {
	f.Extension = filepath.Ext(f.Path)
	f.Filename = strings.TrimSuffix(filepath.Base(f.Path), `.`+f.Extension)
}

var DBConnectionPool *Pool

type dbToken struct{}

type Pool struct {
	sem  chan dbToken
	idle chan *pgx.Conn
}

// inserts the basic values of the file, all need to be changed via updates
func InsertFile(f File) error {
	db := DBConnectionPool.Get()
	defer func() {
		DBConnectionPool.Release(db)
	}()

	_, err := db.Exec(context.Background(), `insert into files (id, filename, extension, path)
		values ($1, $2, $3, $4)`,
		f.ID, f.Filename, f.Extension, f.Path)
	if err != nil {
		fmt.Println("Error during insert", err)
		return err
	}
	return nil
}

func QueryFileByPath(path string) File {
	files, _ := QueryFiles(`files`, `path = $1`, path)
	if len(files) > 0 {
		return files[0]
	}
	return File{Path: path}
}

func QueryFiles(tablename string, where string, args ...interface{}) ([]File, error) {
	db := DBConnectionPool.Get()
	defer func() {
		DBConnectionPool.Release(db)
	}()

	rows, err := db.Query(context.Background(), `select id, filename, extension, path, id3_artist, id3_album_artist, id3_album, id3_title, id3_md5, id3_scanned, deleted, size, md5
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

func (p *Pool) Get() *pgx.Conn {
	select {
	case db := <-p.idle:
		return db
	case p.sem <- dbToken{}:
		db := p.DBConnect()
		return db
	}
}

func (p *Pool) Release(db *pgx.Conn) {
	p.idle <- db
}

func (p *Pool) DBConnect() *pgx.Conn {
	conn, err := pgx.Connect(context.Background(), "postgresql://chris:cvdl@localhost/filescanner")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connection to database: %v\n", err)
		os.Exit(1)
	}
	return conn
}

func InitDBConnectionPool() {
	if DBConnectionPool == nil {
		sem := make(chan dbToken, 10)
		idle := make(chan *pgx.Conn, 10)
		DBConnectionPool = &Pool{sem, idle}
	}
}
