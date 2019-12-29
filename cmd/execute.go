package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	pb "github.com/cheggaaa/pb/v3"
	"github.com/dhowden/tag"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var ExecuteFilePath string
var Action string

type Executioner func(param string)

var Executioners = map[string]Executioner{
	"unlink": unlink,
	"move":   move,
	"read":   read,
}

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
	err := os.Remove(path)
	if err != nil {
		fmt.Printf("Error removing %s: %v", path, err)
	}
}

func move(path string) {
	targetRootDirectory := `/Volumes/music/sorted_music/`
	file := QueryFileByPath(path)
	if file.ID == uuid.Nil && len(path) > 5 {
		fmt.Printf("Cannot find in db: %v\n", path)
		return
	}
	directory := file.ArtistAlbumDirectory()
	if len(directory) == 0 {
		fmt.Printf("Not enough ID tags %v\n", path)
	}

	targetDirectory := targetRootDirectory + strings.Join(directory, `/`)
	targetFile := targetDirectory + `/` + file.Filename

	err := os.MkdirAll(targetDirectory, 0755)
	if err != nil {
		fmt.Println("Error during making of directory", err)
		os.Exit(1)
	}
	newFile, err := os.Create(targetFile)
	if err != nil {
		fmt.Println("error during file create", targetFile, err)
	}
	newFile.Close()
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
	cmd.MarkFlagRequired("path")
	cmd.Flags().StringVarP(&Action, "action", "a", "", "Action to perform on each line")
	return cmd
}
