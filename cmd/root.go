package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "nomad-pipeline",
	Short: "Run pipeline-style workloads in Nomad",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var cPath string

func init() {
	rootCmd.PersistentFlags().StringVar(&cPath, "config", "config.yaml", "path to config")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
