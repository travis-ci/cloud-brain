package background

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

// RedisBackend is a Backend implementation backed by Redis.
type RedisBackend struct {
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

// NewRedisBackend creates a RedisBackend that connects to the given redis.Pool
func NewRedisBackend(pool *redis.Pool, prefix string) *RedisBackend {
	return &RedisBackend{
		pool:   pool,
		prefix: prefix,
	}
}

func (rb *RedisBackend) Enqueue(job Job) error {
	rj := rb.jobToRedisJob(job)

	payload := new(bytes.Buffer)
	err := gob.NewEncoder(payload).Encode(rj)
	if err != nil {
		return err
	}

	conn := rb.pool.Get()
	defer conn.Close()

	conn.Send("MULTI")
	conn.Send("SADD", rb.key("queues"), job.Queue)
	conn.Send("LPUSH", rb.key("queue:"+job.Queue), payload.Bytes())
	_, err = conn.Do("EXEC")

	return err
}

func (rb *RedisBackend) FetchWork(queue string) (Job, error) {
	conn := rb.pool.Get()
	defer conn.Close()

	reply, err := redis.ByteSlices(conn.Do("BRPOP", rb.key("queue:"+queue), "0"))
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

func (rb *RedisBackend) ScheduleAt(t time.Time, job Job) error {
	rj := rb.jobToRedisJob(job)

	payload := new(bytes.Buffer)
	err := gob.NewEncoder(payload).Encode(rj)
	if err != nil {
		return err
	}

	conn := rb.pool.Get()
	defer conn.Close()

	_, err = conn.Do("ZADD", rb.key("schedule"), strconv.FormatInt(t.Unix(), 10), payload.Bytes())
	return err
}

func (rb *RedisBackend) jobToRedisJob(job Job) redisJob {
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

func (rb *RedisBackend) redisJobToJob(rj redisJob) Job {
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

func (rb *RedisBackend) pollAndEnqueue() {
	// Sleep for 0-5 seconds to make sure every process doesn't hit Redis at
	// once, avoiding a thundering herd scenario
	initialWait := time.Duration(rand.Intn(5)) * time.Second
	time.Sleep(initialWait)

	for {
		rb.enqueueJobs(time.Now())

		// TODO(henrikhodne): This should ideally be scaled to be 15*number of
		// workers
		averageSleepSeconds := 15
		time.Sleep(time.Duration(int64(float64(averageSleepSeconds)*rand.Float64()+(float64(averageSleepSeconds)/2.0))) * time.Second)
	}
}

func (rb *RedisBackend) enqueueJobs(now time.Time) {
	conn := rb.pool.Get()
	defer conn.Close()

	nowStr := strconv.FormatInt(now.Unix(), 10)

	for {
		payloads, err := redis.ByteSlices(conn.Do("ZRANGEBYSCORE", rb.key("schedule"), "-inf", nowStr, "LIMIT", "0", "1"))
		if err != nil {
			// TODO(henrikhodne): Log error?
			continue
		}

		if len(payloads) < 1 {
			break
		}

		payload := payloads[0]

		removed, err := redis.Int64(conn.Do("ZREM", rb.key("schedule"), payload))
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

		err = rb.Enqueue(rb.redisJobToJob(rj))
		if err != nil {
			// TODO(henrikhodne): Log error?
			continue
		}
	}
}

func (rb *RedisBackend) key(key string) string {
	if rb.prefix == "" {
		return key
	}

	return rb.prefix + ":" + key
}
