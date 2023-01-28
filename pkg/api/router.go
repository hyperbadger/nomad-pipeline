package api

import (
	"fmt"
	"net/http"
	"time"

	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	nomad "github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/helper"
	"github.com/hashicorp/nomad/helper/uuid"
	"github.com/hyperbadger/nomad-pipeline/pkg/controller"
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
	// allow slashes escaped as %2F in url parameters
	// eg. allow /jobs/parent-job%2Fdispatch-one for
	// looking up the parent-job/dispatch-one job
	r.UseRawPath = true

	desugar := ps.logger.Desugar()

	// logging
	r.Use(ginzap.Ginzap(desugar, time.RFC3339, true))
	r.Use(ginzap.RecoveryWithZap(desugar, true))

	r.GET("/health", ps.health)
	r.GET("/jobs", ps.listAllJobs)
	r.GET("/jobs/:jobID", ps.getJob)
	r.GET("/pipelines", ps.listPipelines)
	r.GET("/pipelines/:pipelineID", ps.getPipeline)
	r.GET("/pipelines/:pipelineID/jobs", ps.listPipelineJobs)
	r.GET("/pipelines/:pipelineID/jobs/:jobID", ps.getJob)

	r.POST("/pipelines/:pipelineID", ps.dispatchPipeline)

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
	jobs, httpErr := ps.listJobs(basicQuery, notParam)
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

func (ps *PipelineServer) getJob(c *gin.Context) {
	pipelineID := c.Params.ByName("pipelineID")
	jobID := c.Params.ByName("jobID")

	jobs, httpErr := ps.listJobs(queryWithPrefix(jobID), notParam)
	if httpErr != nil {
		httpErr.Apply(c, ps.logger)
		return
	}

	filters := []getJobsFilter{isPipeline}
	if len(pipelineID) > 0 {
		filters = append(filters, isChild(pipelineID))
	}

	njobs, httpErr := ps.getJobs(jobs, filters...)
	if httpErr != nil {
		httpErr.Apply(c, ps.logger)
		return
	}

	matchNjobs := make([]NomadJob, 0)

	for _, njob := range njobs {
		if *njob.full.ID == jobID {
			matchNjobs = append(matchNjobs, njob)
		}
	}

	if len(matchNjobs) == 0 {
		httpErr = NewError(
			WithType(ErrorTypeNotFound),
			WithCode(http.StatusNotFound),
			WithMessage(fmt.Sprintf("couldn't find job with id: %v", jobID)),
		)
		httpErr.Apply(c, ps.logger)
		return
	}

	if len(matchNjobs) > 1 {
		httpErr = NewError(
			WithType(ErrorTypeNotFound),
			WithCode(http.StatusNotFound),
			WithMessage(fmt.Sprintf("found multiple jobs with id: %v", jobID)),
		)
		httpErr.Apply(c, ps.logger)
		return
	}

	job, httpErr := ps.newJobFromNomadJob(matchNjobs[0])
	if httpErr != nil {
		httpErr.Apply(c, ps.logger)
		return
	}

	c.JSON(http.StatusOK, job)
}

func (ps *PipelineServer) listPipelines(c *gin.Context) {
	paramJobs, httpErr := ps.listJobs(basicQuery, isParam)
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

func (ps *PipelineServer) getPipeline(c *gin.Context) {
	pipelineID := c.Params.ByName("pipelineID")

	pipeline, httpErr := ps.getPipelineByID(pipelineID)
	if httpErr != nil {
		httpErr.Apply(c, ps.logger)
		return
	}

	c.JSON(http.StatusOK, pipeline)
}

func (ps *PipelineServer) listPipelineJobs(c *gin.Context) {
	pipelineID := c.Params.ByName("pipelineID")

	jobs, httpErr := ps.listJobs(queryWithPrefix(pipelineID), notParam)
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

func (ps *PipelineServer) dispatchPipeline(c *gin.Context) {
	pipelineID := c.Params.ByName("pipelineID")

	pipeline, httpErr := ps.getPipelineByID(pipelineID)
	if httpErr != nil {
		httpErr.Apply(c, ps.logger)
		return
	}

	var disp DispatchPipeline
	err := c.ShouldBindJSON(&disp)
	if err != nil {
		httpErr = NewError(
			WithType(ErrorBadRequest),
			WithCode(http.StatusBadRequest),
			WithError(err),
		)
		httpErr.Apply(c, ps.logger)
		return
	}

	meta := make(map[string]string)

	missingReqVars := make([]string, 0)
	for key, def := range pipeline.RequiredVars {
		if val, ok := disp.Variables[key]; ok {
			meta[key] = val
		} else {
			meta[key] = def
		}

		if len(meta[key]) == 0 {
			missingReqVars = append(missingReqVars, key)
		}
	}

	if len(missingReqVars) > 0 {
		httpErr = NewError(
			WithType(ErrorBadRequest),
			WithCode(http.StatusBadRequest),
			WithMessage(fmt.Sprintf("missing required variables: %v", missingReqVars)),
		)
		httpErr.Apply(c, ps.logger)
		return
	}

	for key, def := range pipeline.OptionalVars {
		if val, ok := disp.Variables[key]; ok {
			meta[key] = val
		} else {
			meta[key] = def
		}
	}

	jobsAPI := ps.nomad.Jobs()

	job := pipeline.RawJob.full

	for k, v := range meta {
		job.SetMeta(k, v)
	}

	job.SetMeta(controller.TagParentPipeline, *job.ID)

	job.ParameterizedJob = nil

	job.Meta = expandMeta(job.Meta)

	for _, tg := range job.TaskGroups {
		tg.Meta = expandMeta(tg.Meta, job.Meta)

		for _, t := range tg.Tasks {
			t.Meta = expandMeta(t.Meta, tg.Meta, job.Meta)
		}
	}

	name := job.Meta[controller.TagName]
	if len(name) == 0 {
		name = fmt.Sprintf("dispatch-%d-%s", time.Now().Unix(), uuid.Short())
	}

	name = helper.CleanEnvVar(name, '-')

	jobID := fmt.Sprintf("%s/%s", *job.ID, name)
	jobName := fmt.Sprintf("%s/%s", *job.Name, name)
	job.ID = &jobID
	job.Name = &jobName

	_, _, err = jobsAPI.Register(pipeline.RawJob.full, &nomad.WriteOptions{})
	if err != nil {
		httpErr = NewError(
			WithType(ErrorTypeNomadUpstream),
			WithMessage("error dispatching job"),
			WithError(err),
		)
		httpErr.Apply(c, ps.logger)
		return
	}
}
