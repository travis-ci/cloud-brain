package worker

import (
	"bytes"
	"encoding/gob"
	"math/rand"
	"strconv"
	"time"

	"golang.org/x/net/context"

	"github.com/garyburd/redigo/redis"
	"github.com/travis-ci/cloud-brain/cbcontext"
)

// RedisWorker is a worker.Backend implementation backed by Redis.
type RedisWorker struct {
	pool   *redis.Pool
	prefix string
}

type redisJob struct {
	UUID       string
	Payload    []byte
	Queue      string
	MaxRetries uint
	RetryCount uint
	Error      error
	FailedAt   time.Time
	RetriedAt  time.Time
}

// NewRedisWorker creates a RedisWorker that connects to the given redis.Pool
func NewRedisWorker(pool *redis.Pool, prefix string) *RedisWorker {
	return &RedisWorker{
		pool:   pool,
		prefix: prefix,
	}
}

func (rw *RedisWorker) Enqueue(job Job) error {
	rj := rw.jobToRedisJob(job)

	payload := new(bytes.Buffer)
	err := gob.NewEncoder(payload).Encode(rj)
	if err != nil {
		return err
	}

	conn := rw.pool.Get()
	defer conn.Close()

	conn.Send("MULTI")
	conn.Send("SADD", rw.key("queues"), job.Queue)
	conn.Send("LPUSH", rw.key("queue:"+job.Queue), payload.Bytes())
	_, err = conn.Do("EXEC")

	return err
}

func (rw *RedisWorker) FetchWork(queue string) (Job, error) {
	conn := rw.pool.Get()
	defer conn.Close()

	reply, err := redis.ByteSlices(conn.Do("BRPOP", rw.key("queue:"+queue), "0"))
	if err != nil {
		return Job{}, err
	}

	payload := reply[1]

	payloadReader := bytes.NewReader(payload)

	var rj redisJob
	err = gob.NewDecoder(payloadReader).Decode(&rj)
	if err != nil {
		return Job{}, err
	}

	ctx := context.TODO()
	if rj.UUID != "" {
		ctx = cbcontext.FromUUID(ctx, rj.UUID)
	}

	return Job{
		Context:    ctx,
		Payload:    rj.Payload,
		Queue:      rj.Queue,
		MaxRetries: rj.MaxRetries,
		RetryCount: rj.RetryCount,
		Error:      rj.Error,
		FailedAt:   rj.FailedAt,
		RetriedAt:  rj.RetriedAt,
	}, nil
}

func (rw *RedisWorker) ScheduleAt(t time.Time, job Job) error {
	rj := rw.jobToRedisJob(job)

	payload := new(bytes.Buffer)
	err := gob.NewEncoder(payload).Encode(rj)
	if err != nil {
		return err
	}

	conn := rw.pool.Get()
	defer conn.Close()

	_, err = conn.Do("ZADD", rw.key("schedule"), strconv.FormatInt(t.Unix(), 10), payload.Bytes())
	return err
}

func (rw *RedisWorker) jobToRedisJob(job Job) redisJob {
	rj := redisJob{
		Payload:    job.Payload,
		Queue:      job.Queue,
		MaxRetries: job.MaxRetries,
		RetryCount: job.RetryCount,
		Error:      job.Error,
		FailedAt:   job.FailedAt,
		RetriedAt:  job.RetriedAt,
	}
	uuid, ok := cbcontext.UUIDFromContext(job.Context)
	if ok {
		rj.UUID = uuid
	}

	return rj
}

func (rw *RedisWorker) redisJobToJob(rj redisJob) Job {
	ctx := context.TODO()
	if rj.UUID != "" {
		ctx = cbcontext.FromUUID(ctx, rj.UUID)
	}

	return Job{
		Context:    ctx,
		Payload:    rj.Payload,
		Queue:      rj.Queue,
		MaxRetries: rj.MaxRetries,
		RetryCount: rj.RetryCount,
		Error:      rj.Error,
		FailedAt:   rj.FailedAt,
		RetriedAt:  rj.RetriedAt,
	}
}

func (rw *RedisWorker) pollAndEnqueue() {
	// Sleep for 0-5 seconds to make sure every process doesn't hit Redis at
	// once, avoiding a thundering herd scenario
	initialWait := time.Duration(rand.Intn(5)) * time.Second
	time.Sleep(initialWait)

	for {
		rw.enqueueJobs(time.Now())

		// TODO(henrikhodne): This should ideally be scaled to be 15*number of
		// workers
		averageSleepSeconds := 15
		time.Sleep(time.Duration(int64(float64(averageSleepSeconds)*rand.Float64()+(float64(averageSleepSeconds)/2.0))) * time.Second)
	}
}

func (rw *RedisWorker) enqueueJobs(now time.Time) {
	conn := rw.pool.Get()
	defer conn.Close()

	nowStr := strconv.FormatInt(now.Unix(), 10)

	for {
		payloads, err := redis.ByteSlices(conn.Do("ZRANGEBYSCORE", rw.key("schedule"), "-inf", nowStr, "LIMIT", "0", "1"))
		if err != nil {
			// TODO(henrikhodne): Log error?
			continue
		}

		if len(payloads) < 1 {
			break
		}

		payload := payloads[0]

		removed, err := redis.Int64(conn.Do("ZREM", rw.key("schedule"), payload))
		if err != nil {
			// TODO(henrikhodne): Log error?
			continue
		}

		if removed == 0 {
			// A different connection scheduled the job already, let's go to the
			// next one.
			continue
		}

		payloadReader := bytes.NewReader(payload)

		var rj redisJob
		err = gob.NewDecoder(payloadReader).Decode(&rj)
		if err != nil {
			// TODO(henrikhodne): Log error?
			continue
		}

		err = rw.Enqueue(rw.redisJobToJob(rj))
		if err != nil {
			// TODO(henrikhodne): Log error?
			continue
		}
	}
}

func (rw *RedisWorker) key(key string) string {
	if rw.prefix == "" {
		return key
	}

	return rw.prefix + ":" + key
}
