 # SQL Statements
 
  * SQL to extract file extension
    `update files f set extension =  ( select lower((regexp_matches(f.path,'\.(\w+)$'))[1]) );`
  * List of file types
    `select extension, count(*) from files group by extension having count(*) > 10 order by count(*);`
  * Create music file view:
    `create view music_files as select * from files where extension in ( 'm4b', 'm4p', 'm4a', 'mp3', 'ogg')`
