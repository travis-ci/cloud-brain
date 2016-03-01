package worker

import (
	"sync"
	"time"
)

type MemoryWorker struct {
	jobsMutex sync.Mutex
	jobs      map[string][]Job
}

func NewMemoryWorker() *MemoryWorker {
	return &MemoryWorker{
		jobs: make(map[string][]Job),
	}
}

func (mw *MemoryWorker) Enqueue(job Job) error {
	mw.jobsMutex.Lock()
	defer mw.jobsMutex.Unlock()

	if _, ok := mw.jobs[job.Queue]; !ok {
		mw.jobs[job.Queue] = make([]Job, 0)
	}

	mw.jobs[job.Queue] = append(mw.jobs[job.Queue], job)
	return nil
}

func (mw *MemoryWorker) FetchWork(queue string) (Job, error) {
	mw.jobsMutex.Lock()
	if _, ok := mw.jobs[queue]; !ok {
		mw.jobs[queue] = make([]Job, 0)
	}
	mw.jobsMutex.Unlock()

	for {
		mw.jobsMutex.Lock()
		if len(mw.jobs[queue]) < 1 {
			mw.jobsMutex.Unlock()
			// A quick sleep to make sure we're not hogging the mutex
			time.Sleep(time.Millisecond)
			continue
		}
		job := mw.jobs[queue][0]
		mw.jobs[queue] = mw.jobs[queue][1:]
		mw.jobsMutex.Unlock()

		return job, nil
	}
}

func (mw *MemoryWorker) ScheduleAt(t time.Time, job Job) error {
	go func() {
		time.Sleep(t.Sub(time.Now()))
		mw.Enqueue(job)
	}()

	return nil
}
