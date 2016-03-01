package cbcontext

import (
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/getsentry/raven-go"
	"golang.org/x/net/context"
)

type contextKey int

const (
	uuidKey contextKey = iota
)

// FromUUID generates a new context with the given context as its parent, and
// stores the given UUID with the context. The UUID can be retrieved again using
// UUIDFromContext.
func FromUUID(ctx context.Context, uuid string) context.Context {
	return context.WithValue(ctx, uuidKey, uuid)
}

// UUIDFromContext returns the UUID stored in the context with FromUUID. If no
// UUID is stored in the context, the second argument is false. Otherwise it is
// true.
func UUIDFromContext(ctx context.Context) (string, bool) {
	uuid, ok := ctx.Value(uuidKey).(string)
	return uuid, ok
}

// LoggerFromContext returns a logrus.Entry with the PID of the current process
// set as a field, and also includes every field set using the From* functions
// this package.
func LoggerFromContext(ctx context.Context) *logrus.Entry {
	entry := logrus.WithField("pid", os.Getpid())

	if uuid, ok := UUIDFromContext(ctx); ok {
		entry = entry.WithField("uuid", uuid)
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
	if uuid, ok := UUIDFromContext(ctx); ok {
		tags["uuid"] = uuid
	}

	packet := raven.NewPacket(
		err.Error(),
		interfaces...,
	)
	raven.DefaultClient.Capture(packet, tags)
}
