package api

import (
	nomad "github.com/hashicorp/nomad/api"
	"github.com/hyperbadger/nomad-pipeline/pkg/controller"
)

type NomadJob struct {
	stub *nomad.JobListStub
	full *nomad.Job
}

func sumTGSummary(tgs nomad.TaskGroupSummary) int {
	sum := tgs.Queued +
		tgs.Complete +
		tgs.Failed +
		tgs.Running +
		tgs.Starting +
		tgs.Lost +
		tgs.Unknown

	return sum
}

type Job struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

func (ps *PipelineServer) newJobFromNomadJob(njob NomadJob) (*Job, *Error) {
	jobsAPI := ps.nomad.Jobs()

	status := njob.stub.Status

	if status == "dead" {
		ftgs := make([]string, 0)

		for tg, tgs := range njob.stub.JobSummary.Summary {
			runs := sumTGSummary(tgs)

			if runs == 1 {
				if tgs.Failed == 1 || tgs.Lost == 1 || tgs.Unknown == 1 {
					ftgs = append(ftgs, tg)
				}
			} else if runs > 1 {
				allocs, _, err := jobsAPI.Allocations(njob.stub.ID, true, &nomad.QueryOptions{})
				if err != nil {
					httpErr := NewError(
						WithType(ErrorTypeNomadUpstream),
						WithMessage("error listing job allocs"),
						WithError(err),
					)
					return nil, httpErr
				}

				if !controller.TgDone(allocs, []string{tg}, true) {
					ftgs = append(ftgs, tg)
				}
			} else {
				ps.logger.Warnw("dead job without any runs", "job", njob.stub.ID, "taskgroup", tg)
			}
		}

		if len(ftgs) > 0 {
			status = "failed"
		}

		status = "success"
	}

	job := Job{
		ID:     njob.stub.ID,
		Name:   njob.stub.Name,
		Status: status,
	}

	return &job, nil
}

type getJobsFilter func(*nomad.Job) bool

func isPipeline(job *nomad.Job) bool {
	if _, ok := job.Meta[controller.TagEnabled]; ok {
		return true
	}

	return false
}

func isChild(ofParent ...string) getJobsFilter {
	return func(job *nomad.Job) bool {
		var parentID string

		if id, ok := job.Meta[controller.TagParentPipeline]; ok {
			parentID = id
		}

		if job.ParentID != nil && len(*job.ParentID) > 0 {
			parentID = *job.ParentID
		}

		if len(ofParent) > 0 {
			return parentID == ofParent[0]
		}

		if len(parentID) > 0 {
			return true
		}

		return false
	}
}

func (ps *PipelineServer) getJobs(njobs []NomadJob, filters ...getJobsFilter) ([]NomadJob, *Error) {
	jobsAPI := ps.nomad.Jobs()

	fjobs := make([]NomadJob, 0)

	for _, njob := range njobs {
		job, _, err := jobsAPI.Info(njob.stub.ID, &nomad.QueryOptions{})
		if err != nil {
			httpErr := NewError(
				WithType(ErrorTypeNomadUpstream),
				WithMessage("error getting job"),
				WithError(err),
			)
			return nil, httpErr
		}

		truthy := 0
		for _, filter := range filters {
			if filter(job) {
				truthy += 1
			}
		}

		if len(filters) == truthy {
			fjobs = append(fjobs, NomadJob{stub: njob.stub, full: job})
		}
	}

	return fjobs, nil
}

type listJobsFilter func(*nomad.JobListStub) bool

func isParam(job *nomad.JobListStub) bool {
	return job.ParameterizedJob
}

func notParam(job *nomad.JobListStub) bool {
	return !job.ParameterizedJob
}

type queryOptions func(*nomad.QueryOptions)

func basicQuery(_ *nomad.QueryOptions) {
}

func queryWithPrefix(prefix string) func(*nomad.QueryOptions) {
	return func(qo *nomad.QueryOptions) {
		qo.Prefix = prefix
	}
}

func (ps *PipelineServer) listJobs(qOpts queryOptions, filters ...listJobsFilter) ([]NomadJob, *Error) {
	jobsAPI := ps.nomad.Jobs()

	qo := &nomad.QueryOptions{}
	qOpts(qo)

	allJobs, _, err := jobsAPI.List(qo)
	if err != nil {
		httpErr := NewError(
			WithType(ErrorTypeNomadUpstream),
			WithMessage("error listing jobs"),
			WithError(err),
		)
		return nil, httpErr
	}

	fjobs := make([]NomadJob, 0)

	for _, job := range allJobs {
		truthy := 0
		for _, filter := range filters {
			if filter(job) {
				truthy += 1
			}
		}

		if len(filters) == truthy {
			fjobs = append(fjobs, NomadJob{stub: job})
		}
	}

	return fjobs, nil
}
