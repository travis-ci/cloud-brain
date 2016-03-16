// Package background contains the tools necessary for running jobs in the
// background.
package background

import (
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/travis-ci/cloud-brain/cbcontext"

	"golang.org/x/net/context"
)

// Backend is the interface that must be implemented by background worker
// backends.
type Backend interface {
	Enqueue(job Job) error
	FetchWork(queue string) (Job, error)
	ScheduleAt(t time.Time, job Job) error
}

// Worker is the interface that must be implemented by something that wants to
// receive background jobs. It's passed to background.Run.
type Worker interface {
	Work(ctx context.Context, payload []byte) error
}

// WorkerFunc implements the Worker interface so regular functions can be used
// as workers. If f is a function with the appropriate signature, WorkerFunc(f)
// is a Worker that calls f for each job.
type WorkerFunc func(ctx context.Context, payload []byte) error

// Work calls the function with the given arguments. This makes WorkerFunc
// implement Worker.
func (wf WorkerFunc) Work(ctx context.Context, payload []byte) error {
	return wf(ctx, payload)
}

type Job struct {
	Context    context.Context
	Payload    []byte
	Queue      string
	MaxRetries uint

	// These are set internally and should be considered read-only. Zero values
	// are okay for new jobs.
	RetryCount uint
	Error      error
	FailedAt   time.Time
	RetriedAt  time.Time
}

// Run will contiuously poll a daemon and run the job on the worker.
func Run(ctx context.Context, queue string, backend Backend, worker Worker) error {
	for {
		job, err := backend.FetchWork(queue)
		if err != nil {
			return err
		}

		err = worker.Work(job.Context, job.Payload)
		if err != nil {
			job.Error = err
			if job.RetryCount > 0 {
				job.RetriedAt = time.Now()
			} else {
				job.FailedAt = time.Now()
			}
			job.RetryCount++

			if job.RetryCount <= job.MaxRetries {
				// TODO(henrikhodne): These numbers probably need to be tweaked.
				delay := time.Duration(job.RetryCount) * 5 * time.Second
				backend.ScheduleAt(time.Now().Add(delay), job)
			} else {
				cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
					"err":       err,
					"retries":   job.RetryCount,
					"failed_at": job.FailedAt,
					"queue":     job.Queue,
				}).Error("exhausted retry count")
			}
		}
	}
}
