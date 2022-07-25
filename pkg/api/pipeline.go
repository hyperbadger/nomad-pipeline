package api

type Pipeline struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (ps *PipelineServer) newPipelineFromNomadJob(njob NomadJob) *Pipeline {
	pipeline := Pipeline{
		ID:   *njob.full.ID,
		Name: *njob.full.Name,
	}

	return &pipeline
}
