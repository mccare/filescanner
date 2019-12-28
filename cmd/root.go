package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"os"
)

var rootCmd = &cobra.Command{
	Use:   "filescanner",
	Short: "Filescanner will try to detect duplicate files based on md5",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Root command")
	},
}

func Execute() {

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(NewInitCommand())
	rootCmd.AddCommand(NewScanCommand())
	rootCmd.AddCommand(NewQueryCommand())
	rootCmd.AddCommand(NewExecuteCommand())
}
