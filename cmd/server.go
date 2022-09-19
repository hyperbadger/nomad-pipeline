package cmd

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"sync"

	nomad "github.com/hashicorp/nomad/api"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/hyperbadger/nomad-pipeline/pkg/api"
	"github.com/hyperbadger/nomad-pipeline/pkg/trigger"
)

var serverCmd = &cobra.Command{
	Use: "server",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()

		ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
		defer stop()

		_logger, _ := zap.NewProduction()
		defer _logger.Sync()

		logger := _logger.Sugar()
		logger = logger.With("@module", "server")

		nClient, err := nomad.NewClient(&nomad.Config{
			Address: os.Getenv("NOMAD_ADDR"),
		})
		if err != nil {
			logger.Fatalw("error creating nomad client", "error", err)
		}

		var managers sync.WaitGroup

		tm, err := trigger.NewTriggerManager(ctx, tPath, nClient, logger.With("@module", "trigger"))
		if err != nil {
			logger.Fatalw("error initializing trigger manager", "error", err)
		}

		managers.Add(1)
		go func() {
			defer managers.Done()
			err = tm.ListenAndDispatch(ctx)
			if err != nil {
				logger.Errorw("trigger manager no longer listening due to error", "error", err)
			}
		}()

		ps := api.NewPipelineServer(nClient, logger.With("@module", "pipeline_server"))

		srv := ps.NewHTTPServer(addr)

		srvConnsClosed := make(chan struct{})
		go func() {
			<-ctx.Done()

			// TODO: use context with timeout so that there is a limit on how long
			// we wait for shutting down the server?
			if err := srv.Shutdown(context.TODO()); err != nil {
				logger.Info("server shutdown")
			}
			close(srvConnsClosed)
		}()

		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			logger.Fatalw("server errored", "error", err)
		}

		<-srvConnsClosed
		managers.Wait()
	},
}

var addr string
var tPath string

func init() {
	serverCmd.Flags().StringVar(&addr, "addr", "127.0.0.1:4656", "address server will listen on")
	serverCmd.Flags().StringVar(&tPath, "triggers-file", "triggers.yaml", "path to triggers file")

	rootCmd.AddCommand(serverCmd)
}
