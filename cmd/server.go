package cmd

import (
	"github.com/spf13/cobra"

	"github.com/hyperbadger/nomad-pipeline/pkg/api"
	"go.uber.org/zap"
)

var serverCmd = &cobra.Command{
	Use: "server",
	Run: func(cmd *cobra.Command, args []string) {
		_logger, _ := zap.NewProduction()
		defer _logger.Sync()

		logger := _logger.Sugar()

		ps, err := api.NewPipelineServer(logger)
		if err != nil {
			logger.Fatalf("error creating pipeline server: %w", err)
		}

		srv := ps.NewHTTPServer(addr)

		if err := srv.ListenAndServe(); err != nil {
			logger.Fatalf("server errored: %v", err)
		}
	},
}

var addr string

func init() {
	serverCmd.Flags().StringVar(&addr, "addr", "127.0.0.1:4656", "address server will listen on")

	rootCmd.AddCommand(serverCmd)
}
