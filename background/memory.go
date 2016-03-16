package background

import (
	"sync"
	"time"
)

// MemoryBackend is an implementation of Backend that keeps all its data in
// memory. Useful for testing.
type MemoryBackend struct {
	jobsMutex sync.Mutex
	jobs      map[string][]Job
}

// NewMemoryBackend created a new empty MemoryBackend
func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{
		jobs: make(map[string][]Job),
	}
}

// Enqueue pushes a job onto the given queue. Never returns an error.
func (mb *MemoryBackend) Enqueue(job Job) error {
	mb.jobsMutex.Lock()
	defer mb.jobsMutex.Unlock()

	if _, ok := mb.jobs[job.Queue]; !ok {
		mb.jobs[job.Queue] = make([]Job, 0)
	}

	mb.jobs[job.Queue] = append(mb.jobs[job.Queue], job)
	return nil
}

// FetchWork blocks until a job is available on the given queue, then returns
// that job. Never returns an error.
func (mb *MemoryBackend) FetchWork(queue string) (Job, error) {
	mb.jobsMutex.Lock()
	if _, ok := mb.jobs[queue]; !ok {
		mb.jobs[queue] = make([]Job, 0)
	}
	mb.jobsMutex.Unlock()

	for {
		mb.jobsMutex.Lock()
		if len(mb.jobs[queue]) < 1 {
			mb.jobsMutex.Unlock()
			// A quick sleep to make sure we're not hogging the mutex
			time.Sleep(time.Millisecond)
			continue
		}
		job := mb.jobs[queue][0]
		mb.jobs[queue] = mb.jobs[queue][1:]
		mb.jobsMutex.Unlock()

		return job, nil
	}
}

// ScheduleAt enqueues the given job at the given time. Never returns an error.
func (mb *MemoryBackend) ScheduleAt(t time.Time, job Job) error {
	go func() {
		time.Sleep(t.Sub(time.Now()))
		mb.Enqueue(job)
	}()

	return nil
}
