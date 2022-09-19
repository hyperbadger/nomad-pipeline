package api

import (
	"net/http"
	"time"

	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	nomad "github.com/hashicorp/nomad/api"
	"go.uber.org/zap"
)

type PipelineServer struct {
	nomad  *nomad.Client
	logger *zap.SugaredLogger
}

func NewPipelineServer(nClient *nomad.Client, logger *zap.SugaredLogger) *PipelineServer {
	ps := PipelineServer{
		nomad:  nClient,
		logger: logger,
	}

	return &ps
}

func (ps *PipelineServer) NewHTTPServer(addr string) *http.Server {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()

	desugar := ps.logger.Desugar()

	// logging
	r.Use(ginzap.Ginzap(desugar, time.RFC3339, true))
	r.Use(ginzap.RecoveryWithZap(desugar, true))

	r.GET("/health", ps.health)
	r.GET("/jobs", ps.listAllJobs)
	r.GET("/pipelines", ps.listPipelines)
	r.GET("/pipelines/:pipelineID/jobs", ps.listPipelineJobs)

	srv := http.Server{
		Addr:    addr,
		Handler: r,
	}

	return &srv
}

func (ps *PipelineServer) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"healthy": true})
}

func (ps *PipelineServer) listAllJobs(c *gin.Context) {
	jobs, httpErr := ps.listJobs(notParam)
	if httpErr != nil {
		httpErr.Apply(c, ps.logger)
		return
	}

	njobs, httpErr := ps.getJobs(jobs, isPipeline)
	if httpErr != nil {
		httpErr.Apply(c, ps.logger)
		return
	}

	allJobs := make([]*Job, 0)

	for _, njob := range njobs {
		job, httpErr := ps.newJobFromNomadJob(njob)
		if httpErr != nil {
			httpErr.Apply(c, ps.logger)
		}

		allJobs = append(allJobs, job)
	}

	c.JSON(http.StatusOK, allJobs)
}

func (ps *PipelineServer) listPipelines(c *gin.Context) {
	paramJobs, httpErr := ps.listJobs(isParam)
	if httpErr != nil {
		httpErr.Apply(c, ps.logger)
		return
	}

	njobs, httpErr := ps.getJobs(paramJobs, isPipeline)
	if httpErr != nil {
		httpErr.Apply(c, ps.logger)
		return
	}

	pipelines := make([]*Pipeline, 0)

	for _, job := range njobs {
		pipelines = append(pipelines, ps.newPipelineFromNomadJob(job))
	}

	c.JSON(http.StatusOK, pipelines)
}

func (ps *PipelineServer) listPipelineJobs(c *gin.Context) {
	pipelineID := c.Params.ByName("pipelineID")

	jobs, httpErr := ps.listJobs(notParam)
	if httpErr != nil {
		httpErr.Apply(c, ps.logger)
		return
	}

	njobs, httpErr := ps.getJobs(jobs, isPipeline, isChild(pipelineID))
	if httpErr != nil {
		httpErr.Apply(c, ps.logger)
		return
	}

	allJobs := make([]*Job, 0)

	for _, njob := range njobs {
		job, httpErr := ps.newJobFromNomadJob(njob)
		if httpErr != nil {
			httpErr.Apply(c, ps.logger)
		}

		allJobs = append(allJobs, job)
	}

	c.JSON(http.StatusOK, allJobs)
}
