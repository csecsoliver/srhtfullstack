package worker

import (
	"sync"
)

type Jobs struct {
	jobs  map[int]*JobContext
	mutex sync.Mutex
}

func NewJobs() *Jobs {
	return &Jobs{
		jobs: make(map[int]*JobContext, 1),
	}
}

func (j *Jobs) Get(jobID int) (*JobContext, bool) {
	job, ok := j.jobs[jobID]
	return job, ok
}

func (j *Jobs) SetJobContext(jobID int, ctx *JobContext) {
	j.mutex.Lock()
	j.jobs[jobID] = ctx
	j.mutex.Unlock()
}

func (j *Jobs) DeleteJobContext(jobID int) {
	j.mutex.Lock()
	delete(j.jobs, jobID)
	j.mutex.Unlock()
}

func (j *Jobs) With(jobID int, do func(*JobContext)) bool {
	j.mutex.Lock()
	defer j.mutex.Unlock()

	job, ok := j.jobs[jobID]
	if ok {
		do(job)
	}
	return ok
}
