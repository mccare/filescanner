package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pb "github.com/cheggaaa/pb/v3"
	"github.com/dhowden/tag"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var ExecuteFilePath string
var Action string
var DryRun bool

type Executioner func(param string)

var Executioners = map[string]Executioner{
	"unlink":   unlink,
	"moveID3":  moveID3,
	"movePath": movePath,
	"read":     read,
}

var TargetRootDirectory string = `/Volumes/music/untagged_music/`

func read(path string) {
	f, err := os.Open(path)
	if err != nil {
		fmt.Println("Error during open", path, err)
		return
	}
	m, _ := tag.ReadFrom(f)

	output, _ := json.MarshalIndent(m.Raw(), "", "  ")
	fmt.Print(string(output))
}

func unlink(path string) {
	if DryRun {
		fmt.Println("Removing", path)
		return
	}
	err := os.Remove(path)
	if err != nil {
		fmt.Printf("Error removing %s: %v", path, err)
	}
}

func movePath(path string) {
	moveGeneric(path, false)
}

func moveID3(path string) {
	moveGeneric(path, true)
}

func moveGeneric(path string, useID3Tags bool) {
	file := QueryFileByPath(path)
	if file.ID == uuid.Nil || file.Deleted {
		fmt.Printf("Cannot find in db: %v\n", path)
		return
	}

	var directory []string
	if useID3Tags {
		directory = file.ArtistAlbumDirectory()
		if len(directory) == 0 {
			fmt.Printf("Not enough ID tags %v\n", path)
		}
	} else {
		fileDir := filepath.Dir(path)
		previousDir := filepath.Dir(fileDir)
		directory = append(directory, filepath.Base(previousDir), filepath.Base(fileDir))
	}

	targetDirectory := TargetRootDirectory + strings.Join(directory, `/`)
	targetFile := targetDirectory + `/` + file.Filename + `.` + file.Extension

	if DryRun {
		fmt.Println("Moving to", targetFile)
		return
	}

	err := os.MkdirAll(targetDirectory, 0755)
	if err != nil {
		fmt.Println("Error during making of directory", err)
		os.Exit(1)
	}

	if path == targetFile {
		fmt.Println("Nothing to do, target file is equal to existing location", targetFile)
		return
	}

	err = os.Rename(path, targetFile)
	if err != nil {
		fmt.Println("error during move", targetFile, err)
		os.Exit(1)
	}
	db := DBConnectionPool.Get()
	db.Exec(context.Background(), "update files set path = $1 where id = $2", targetFile, file.ID)
	DBConnectionPool.Release(db)

}

func execute(maxExecutioners int, executer Executioner) {
	file, err := os.Open(ExecuteFilePath)
	if err != nil {
		fmt.Println("error during opening", err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	var lines []string
	for {
		line, err := reader.ReadString('\n')
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSpace(line)
		lines = append(lines, line)
		if err != nil {
			break
		}
	}
	bar := pb.StartNew(len(lines))
	sem := make(chan token, maxExecutioners)
	for _, i := range lines {
		bar.Increment()
		sem <- token{}
		go func(path string) {
			executer(path)
			<-sem
		}(i)
	}
	for i := 0; i < maxExecutioners; i++ {
		sem <- token{}
	}
}

func print(path string) {
	fmt.Println(path)
}

func NewExecuteCommand() *cobra.Command {
	InitDBConnectionPool()
	cmd := &cobra.Command{
		Use:   "execute",
		Short: "execute action on a list",
		Run: func(cmd *cobra.Command, args []string) {
			action := Executioners[Action]
			if action == nil {
				action = print
			}
			execute(50, action)
		},
	}
	cmd.Flags().StringVarP(&ExecuteFilePath, "path", "p", "", "Path to the file")
	cmd.Flags().BoolVar(&DryRun, "dry-run", false, "do not perform the action, just print out some information for the action")
	cmd.MarkFlagRequired("path")
	cmd.Flags().StringVarP(&Action, "action", "a", "", "Action to perform on each line")
	return cmd
}
