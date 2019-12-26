package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var ExecuteFilePath string
var Action string

type Executioner func(param string)

var Executioners = map[string]Executioner{
	"unlink": unlink,
}

func unlink(path string) {
	err := os.Remove(path)
	if err != nil {
		fmt.Printf("Error removing %s: %v", path, err)
	}
}

func execute(executer Executioner) {
	file, err := os.Open(ExecuteFilePath)
	if err != nil {
		fmt.Println("error during opening", err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	var lines []string
	for {
		line, err := reader.ReadString('\n')
		lines = append(lines, strings.TrimSuffix(line, "\n"))
		if err != nil {
			break
		}
	}
	for _, i := range lines {
		executer(i)
	}
}

func print(path string) {
	fmt.Println(path)
}

func NewExecuteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "execute",
		Short: "execute action on a list",
		Run: func(cmd *cobra.Command, args []string) {
			action := Executioners[Action]
			if action == nil {
				action = print
			}
			execute(action)
		},
	}
	cmd.Flags().StringVarP(&ExecuteFilePath, "path", "p", "", "Path to the file")
	cmd.MarkFlagRequired("path")
	cmd.Flags().StringVarP(&Action, "action", "a", "", "Action to perform on each line")
	return cmd
}
