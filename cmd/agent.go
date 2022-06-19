package cmd

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/hyperbadger/nomad-pipeline/pkg/controller"
)

var agentCmd = &cobra.Command{
	Use: "agent",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var agentInitCmd = &cobra.Command{
	Use:  "init",
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		pc := controller.NewPipelineController(cPath)

		update := pc.Init()
		if update {
			err := pc.UpdateJob()
			if err != nil {
				log.Fatalf("error updating job: %v", err)
			}
		}
	},
}

var agentWaitCmd = &cobra.Command{
	Use:  "wait",
	Args: cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		pc := controller.NewPipelineController(cPath)

		pc.Wait(args)
	},
}

var agentNextCmd = &cobra.Command{
	Use:  "next",
	Args: cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		pc := controller.NewPipelineController(cPath)

		update := pc.Next(args, dynamicTasks)
		if update {
			err := pc.UpdateJob()
			if err != nil {
				log.Fatalf("error updating job: %v", err)
			}
		}
	},
}

var dynamicTasks string

func init() {
	agentNextCmd.Flags().StringVar(&dynamicTasks, "dynamic-tasks", "", "glob of task files relative to alloc dir")

	agentCmd.AddCommand(agentInitCmd)
	agentCmd.AddCommand(agentWaitCmd)
	agentCmd.AddCommand(agentNextCmd)

	rootCmd.AddCommand(agentCmd)
}
