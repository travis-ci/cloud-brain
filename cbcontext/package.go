package cbcontext

import (
	"context"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/getsentry/raven-go"
)

type contextKey int

type WorkContext struct {
	Context context.Context
}

const (
	requestIDKey contextKey = iota
)

// FromRequestID generates a new context with the given context as its parent,
// and stores the given ID with the context. The ID can be retrieved again
// using RequestIDFromContext. If this is the beginning of the request, it is inherited
// from the X-Request-ID from Heroku.
func FromRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// RequestIDFromContext returns the Request ID stored in the context with
// FromRequestID. If no RequestID is stored in the context, the second argument
// is false. Otherwise it is true.
func RequestIDFromContext(ctx context.Context) (string, bool) {
	requestID, ok := ctx.Value(requestIDKey).(string)
	return requestID, ok
}

// LoggerFromContext returns a logrus.Entry with the PID of the current process
// set as a field, and also includes every field set using the From* functions
// this package.
func LoggerFromContext(ctx context.Context) *logrus.Entry {
	entry := logrus.WithField("pid", os.Getpid())

	if requestID, ok := RequestIDFromContext(ctx); ok {
		entry = entry.WithField("request_id", requestID)
	}

	return entry
}

// CaptureError takes an error and captures the details about it and sends it
// off to Sentry, if Sentry has been set up.
func CaptureError(ctx context.Context, err error) {
	if raven.DefaultClient == nil {
		// No client, so we can short-circuit to make things faster
		return
	}

	interfaces := []raven.Interface{
		raven.NewException(err, raven.NewStacktrace(1, 3, []string{"github.com/travis-ci/cloud-brain"})),
	}

	tags := make(map[string]string)
	if requestID, ok := RequestIDFromContext(ctx); ok {
		tags["requestID"] = requestID
	}

	packet := raven.NewPacket(
		err.Error(),
		interfaces...,
	)
	raven.DefaultClient.Capture(packet, tags)
}
