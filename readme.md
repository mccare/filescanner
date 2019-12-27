 # Find duplicates of sound files

 My first go project and the mission is, to sort through my 100,000 mp3 files in different places and find duplicates and missing files from my main library.

The scan phase will populate a postgres DB with
* filename
* size
* md5 hash
* ID3 tags 

MD5 hash is only calculated if there is a file with the same size already in the DB.

# Usage

## Setup

* `brew install postgres`
* create your files db
* edit code and change the postgres URL (the user is your unix user)
* compile and 
* `filescanner init`

## Usage

### Read in all files
```
filescanner scan -p <path>
```
Will populate the DB with the files and if necessary their MD5 hashes. You need to run this twice (since the MD5 generation is lazy and only will be kicked of for the current file)

### Scan the ID Tags
```
filescanner scan -p <path> -s
```
Will read and fill the ID tags. If they already have been scanned the file will be skipped.

### Sync the DB with the Filesystem
If you delete something on the filesystem, resync the db with
```
filescanner scan -p <path> -c
```

# Finding duplicates

This is a manual process. Some SQL statements are below. First steps to automate some things are in query.go. You can delete/process files with execute.go. 

# Tech I liked
* Cobra for building the command line (http://github.com/spf13/cobra)
* ID3 Tag reader (http://github.com/dhowden/tag)
* PGX Postgres Driver (github.com/cheggaaa/pb/v3)
* Concurrency with channels (see scan.go)

 
## SQL Statements

  * SQL to extract file extension
    `update files f set extension =  ( select lower((regexp_matches(f.path,'\.(\w+)$'))[1]) );`
  * List of file types
    `select extension, count(*) from files group by extension having count(*) > 10 order by count(*);`
  * Create music file view:
    `create or replace view music_files as select * from files where extension in ( 'm4b', 'm4p', 'm4a', 'mp3', 'ogg') and not deleted`

## Find Duplicates in different locations (via SQL)
  * Find duplicates in different directories 
    ```sql
    select path 
      from music_files 
      where 
        path like '/Volumes/music/from_harddisks/%' 
        and md5 in 
          (select md5 from music_files where path like '/Volumes/music/CVDL/%') 
        and md5 is not null
    ```
  * Find duplicates in one directory
    ```sql
      select path, md5 
        from music_files 
        where 
          path like '/Users/chris/Music/%' 
          and md5 in (select md5 from music_files where path like '/Users/chris/Music/%'  group by md5 having count(id) > 1) 
        order by md5
    ```
    
## Find Missing files
  * Files with MD5
  `select path from music_files where path like '/Volumes/music/from_harddisks/%' and md5 not in (select md5 from music_files where path like '/Users/chris/Music/%') and md5 is not null `
  * Find unique files (having no md5 means no other file with the same size exists and therefore must  be unique)
    `select count(path), md5 is null from music_files where path like '/Volumes/music/%' group by md5 is null;`

## Find duplicates by size and same ID3 tags
* 
```sql
  select f2.path from 
      files f1, files f2 
    where 
      f1.size = f2.size 
      and f1.id != f2.id 
      and f1.path like '/Users/chris/Music/%'
      and f2.path like '/Volumes/music/from_harddisks/%'
      and f1.id3_artist = f2.id3_artist
      and f1.id3_album = f2.id3_album
      and f1.id3_title = f2.id3_title
      and f1.id3_album_artist = f2.id3_album_artist
      and (f1.id3_title is not null or f1.id3_album is not null or f1.id3_artist is not null or f1.id3_album_artist is not null)
    ;
```
