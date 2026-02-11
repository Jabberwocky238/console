package k8s

import (
	"log"
)

type Processor struct {
	JobQueue chan Job
	PoolSize int
}

type JobType string

type Job interface {
	Type() JobType
	ID() string
	Do() error
}

func NewProcessor(queueSize int, poolSize int) *Processor {
	return &Processor{
		JobQueue: make(chan Job, queueSize),
		PoolSize: poolSize,
	}
}

func (p *Processor) Close() error {
	close(p.JobQueue)
	log.Println("[processor] stopped")
	return nil
}

func (p *Processor) Submit(job Job) {
	p.JobQueue <- job
}

func (p *Processor) Start() {
	for range p.PoolSize {
		go func() {
			for job := range p.JobQueue {
				if err := job.Do(); err != nil {
					log.Printf("[processor] job failed (type=%s, id=%s): %v", job.Type(), job.ID(), err)
				}
			}
		}()
	}
	log.Println("[processor] started")
}
