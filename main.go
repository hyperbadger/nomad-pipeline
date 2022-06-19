package main

import (
	"os"

	log "github.com/sirupsen/logrus"

	"github.com/hyperbadger/nomad-pipeline/cmd"
)

func init() {
	if len(os.Getenv("NOMAD_PIPELINE_LOG_JSON")) > 0 {
		log.SetFormatter(&log.JSONFormatter{})
	}

	if len(os.Getenv("NOMAD_PIPELINE_DEBUG")) > 0 {
		log.SetLevel(log.DebugLevel)
	}
}

func main() {
	cmd.Execute()
}
