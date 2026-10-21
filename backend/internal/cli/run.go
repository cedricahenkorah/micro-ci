/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run <path>",
	Short: "Queue a pipeline run for a local repository's current commit",
	Long: `Read the branch and current commit from a local Git repository,
then submit a run to the micro-ci API. Use . for the current directory.

The worker executes the commit in a separate workspace. Uncommitted changes
are excluded, and no GitHub push is required. The API and worker must be running.`,
	Example: "  micro-ci run .\n  micro-ci run /path/to/project",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("run called")
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// runCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// runCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
