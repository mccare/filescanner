 # Find duplicates of sound files

 My first go project and the mission is, to sort through my 100,000 mp3 files in different places and find duplicates and missing files from my main library.

The scan phase will populate a postgres DB with
* filename
* size
* md5 hash
* ID3 tags 
* MD5 hash of only the music data (calculated by the ID3 library)


# Setup

* `brew install postgres`
* create your files db
* edit code and change the postgres URL (the user is your unix user)
* compile and 
* `filescanner init`

# Usage

## Read in all files
```
filescanner scan -p <path>
```
Will populate the DB with the files and if necessary their MD5 and ID3 hashes. You need to run this twice (since the MD5 generation is lazy and only will be kicked of for the current file)

## Scan the ID Tags
```
filescanner scan -p <path> -s
```
Will read the ID3 tags and update the databse. If they already have been scanned the file will be skipped.

## Sync the DB with the Filesystem
If you delete something on the filesystem, resync the db with
```
filescanner scan -p <path> -c
```

## Finding duplicats by Query and then process (Move, delete)

Finding duplicates is a multi stage manual process. See the sample SQL snippets below to get some ideas. Some outputs I used directly to move or delete, some outputs I checked before processing them.

First, query the DB producing a list of filenames (paths). Then, use the resulting list of files and pass them to the execute command.

* create a file query.txt with a select query for path names
* run `filescanner query -p ./query.txt > /tmp/output`
* inspect /tmp/output if this is a reasonable set of answers
* run `filescanner execute -p /tmp/output -a unlink --dry-run` to e.g. remove all files 
* edit execute.go to change the destination directory for moving commands
  * other actions are: 
     * moveID3: put files into Artist/Album or _Compilation/Album, --dry-run will produce a list of target files
     * movePath: just move the files with the last two path components to a different directory (e.g. for untagged files)
     * read (just show some ID3 tags for debugging)


# Tech I liked
* Cobra for building the command line (http://github.com/spf13/cobra)
* ID3 Tag reader (http://github.com/dhowden/tag)
* PGX Postgres Driver (github.com/cheggaaa/pb/v3)
* Concurrency with channels (see scan.go, and Bryan Mills at Gophercon 2018, https://www.youtube.com/watch?v=5zXAHh5tJqQ)
* Connection Pool see db.go


# SQL Statements

## Helpful Views, other tidbits

  * SQL to extract file extension
    `update files f set extension =  ( select lower((regexp_matches(f.path,'\.(\w+)$'))[1]) );`
    `update files f set filename =  ( select lower((regexp_matches(f.path,'\/([^\/]+)$'))[1]) );`
  * List of file types
    `select extension, count(*) from files group by extension having count(*) > 10 order by count(*);`
  * Create music file view:
    `create or replace view music_files as select * from files where extension in ( 'm4b', 'm4p', 'm4a', 'mp3', 'ogg') and not deleted`
  * Create view for heap music
    `create or replace view backup_music_files as select * from files where extension in ( 'm4b', 'm4p', 'm4a', 'mp3', 'ogg') and not deleted and path like '/Volumes/music/from%';`

    
## Find Duplicates 

### Find duplicates in different directories 
```sql
    select path 
      from music_files 
      where 
        path like '/Volumes/music/from_harddisks/%' 
        and md5 in 
          (select md5 from music_files where path like '/Volumes/music/CVDL/%') 
        and md5 is not null
```
## Find duplicates in one directory
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
```sql
    select path from music_files 
    where 
      path like '/Volumes/music/from_harddisks/%' 
      and md5 not in (select md5 from music_files where path like '/Users/chris/Music/%') 
      and md5 is not null 
```
  * Find unique files (having no md5 means no other file with the same size exists and therefore must  be unique)
```sql
    select count(path), md5 is null 
      from music_files 
      where 
        path like '/Volumes/music/%' 
      group by md5 is null;
```

## Find duplicates by size and same ID3 tags

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

## Find duplicates by filename and ID3 tags
* Album, artist and title need to be the same (and filename)
```sql
  select f2.path from 
      music_files f1, music_files f2 
    where 
      f1.path like '/Users/chris/Music/%'
      and f2.path like '/Volumes/music/from_harddisks/%'
      and f1.id3_artist = f2.id3_artist
      and f1.id3_album = f2.id3_album
      and f1.id3_title = f2.id3_title
      and (length(f1.id3_title) > 1)
      and (length(f1.id3_album) > 1)
      and (length(f1.id3_artist) > 1)
      and (f1.filename = f2.filename)
    ;
```

* Title, Artist and filename is the same (ignore album and album artist)
```sql
  select f2.path from 
      music_files f1, music_files f2 
    where 
      f1.path like '/Users/chris/Music/%'
      and f2.path like '/Volumes/music/from_harddisks/%'
      and f1.id3_artist = f2.id3_artist
      and f1.id3_title = f2.id3_title
      and (length(f1.id3_title) > 1)
      and (length(f1.id3_artist) > 1)
      and (f1.filename = f2.filename)
    ;
```

* Title and Artist are the same 
```sql
  select f2.path from 
      music_files f1, music_files f2 
    where 
      f1.path like '/Users/chris/Music/%'
      and f2.path like '/Volumes/music/from_harddisks/%'
      and f1.id3_artist = f2.id3_artist
      and f1.id3_title = f2.id3_title
      and (length(f1.id3_title) > 1)
      and (length(f1.id3_artist) > 1)
    ;
```

### Same Filename (review the list before deleting!)
```sql
select f2.path from 
      music_files f1, music_files f2 
    where 
      f1.path like '/Users/chris/Music/%'
      and f2.path like '/Volumes/music/from_harddisks/%'
      and (f1.filename = f2.filename)
      and not f1.filename  like '%intro%'
      and not f1.filename like '%titel%'
    order by f2.path;
```


## Moving files into a artist/album structure

* Finding files that are probably part of a compilation (artists on the album more than two)
```sql
select f.id3_album, f.id3_artist 
from backup_music_files f 
where 
  id3_album in 
    (select id3_album 
      from backup_music_files 
      where 
        id3_album != '' and 
        id3_album_artist = '' 
      group by id3_album 
      having count(distinct(id3_artist)) > 2) 
order by f.id3_album, f.id3_artist;
```

Special album names that are false results include "Greatest Hits", "Unplugged", "Live", "Best of"


 