 # SQL Statements

  * SQL to extract file extension
    `update files f set extension =  ( select lower((regexp_matches(f.path,'\.(\w+)$'))[1]) );`
  * List of file types
    `select extension, count(*) from files group by extension having count(*) > 10 order by count(*);`
  * Create music file view:
    `create view music_files as select * from files where extension in ( 'm4b', 'm4p', 'm4a', 'mp3', 'ogg') and deleted is null`

  * Find unique files (having no md5 means no other file with the same size exists and therefore must  be unique)
    `select count(path), md5 is null from music_files where path like '/Volumes/music/%' group by md5 is null;`
  * Find duplicates (what is in harddisks what is also in music/CVDL)
    `select path from music_files where path like '/Volumes/music/from_harddisks/%' and md5 in (select md5 from music_files where path like '/Volumes/music/CVDL/%') and md5 is not null`
    `select path from music_files where path like '/Volumes/music/from_harddisks/%' and md5 in (select md5 from music_files where path like '/Users/chris/Music/%') and md5 is not null`
  * Find duplicates in one directory
    * `select path, md5 from music_files where path like '/Users/chris/Music/%' and md5 in (select md5 from music_files where path like '/Users/chris/Music/%'  group by md5 having count(id) > 1) order by md5`
    
