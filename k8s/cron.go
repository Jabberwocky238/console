package k8s

import (
	"log"
	"time"
)

type CronScheduler struct {
	processor      *Processor
	durationToCall map[time.Duration][]Job
	timerMap       map[time.Duration]*time.Ticker
	stopCh         chan struct{}
}

func NewCronScheduler(proc *Processor) *CronScheduler {
	return &CronScheduler{
		processor:      proc,
		durationToCall: make(map[time.Duration][]Job),
		timerMap:       make(map[time.Duration]*time.Ticker),
		stopCh:         make(chan struct{}),
	}
}

func (s *CronScheduler) RegisterJob(duration time.Duration, job Job) {
	s.durationToCall[duration] = append(s.durationToCall[duration], job)
}

// Start launches one goroutine per unique duration.
// Jobs with the same duration share a single Ticker.
func (s *CronScheduler) Start() {
	for duration, jobs := range s.durationToCall {
		ticker := time.NewTicker(duration)
		s.timerMap[duration] = ticker

		go s.runTicker(ticker, jobs)
	}
	log.Printf("[cron] started %d ticker(s)", len(s.timerMap))
}

func (s *CronScheduler) runTicker(ticker *time.Ticker, jobs []Job) {
	for {
		select {
		case <-ticker.C:
			for _, job := range jobs {
				s.processor.Submit(job)
			}
		case <-s.stopCh:
			ticker.Stop()
			return
		}
	}
}

func (s *CronScheduler) Stop() {
	close(s.stopCh)
}
