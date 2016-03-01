// Package worker implements the background worker portions of Cloud Brain
package worker

import (
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/travis-ci/cloud-brain/cbcontext"

	"golang.org/x/net/context"
)

type Backend interface {
	Enqueue(job Job) error
	FetchWork(queue string) (Job, error)
	ScheduleAt(t time.Time, job Job) error
}

type Worker interface {
	Work(ctx context.Context, payload []byte) error
}

type WorkerFunc func(ctx context.Context, payload []byte) error

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
