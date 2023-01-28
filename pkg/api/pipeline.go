package api

import (
	"fmt"
	"net/http"
)

type Pipeline struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	RequiredVars map[string]string `json:"required_vars"`
	OptionalVars map[string]string `json:"optional_vars"`
	Payload      bool              `json:"payload"`
	RawJob       *NomadJob         `json:"-"`
}

type DispatchPipeline struct {
	Variables map[string]string `json:"vars"`
	Payload   string            `json:"payload"`
}

func (ps *PipelineServer) newPipelineFromNomadJob(njob NomadJob) *Pipeline {
	pipe := Pipeline{
		ID:           *njob.full.ID,
		Name:         *njob.full.Name,
		RequiredVars: make(map[string]string),
		OptionalVars: make(map[string]string),
		RawJob:       &njob,
	}

	for _, v := range njob.full.ParameterizedJob.MetaRequired {
		dv := njob.full.Meta[v]
		pipe.RequiredVars[v] = dv
	}

	for _, v := range njob.full.ParameterizedJob.MetaOptional {
		dv := njob.full.Meta[v]
		pipe.OptionalVars[v] = dv
	}

	payloadTruthy := []string{"required", "optional"}
	for _, truthy := range payloadTruthy {
		if njob.full.ParameterizedJob.Payload == truthy {
			pipe.Payload = true
		}
	}

	return &pipe
}

func (ps *PipelineServer) getPipelineByID(ID string) (*Pipeline, *Error) {
	paramJobs, httpErr := ps.listJobs(queryWithPrefix(ID), isParam)
	if httpErr != nil {
		return nil, httpErr
	}

	njobs, httpErr := ps.getJobs(paramJobs, isPipeline)
	if httpErr != nil {
		return nil, httpErr
	}

	pipelines := make([]*Pipeline, 0)

	for _, job := range njobs {
		if *job.full.ID == ID {
			pipelines = append(pipelines, ps.newPipelineFromNomadJob(job))
		}
	}

	if len(pipelines) == 0 {
		httpErr = NewError(
			WithType(ErrorTypeNotFound),
			WithCode(http.StatusNotFound),
			WithMessage(fmt.Sprintf("couldn't find pipeline with id: %v", ID)),
		)
		return nil, httpErr
	}

	if len(pipelines) > 1 {
		httpErr = NewError(
			WithType(ErrorTypeNotFound),
			WithCode(http.StatusNotFound),
			WithMessage(fmt.Sprintf("found multiple pipelines with id: %v", ID)),
		)
		return nil, httpErr
	}

	return pipelines[0], nil
}
