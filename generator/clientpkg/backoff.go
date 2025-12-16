package clientpkg

import (
	"context"
	"log/slog"

	backoff "github.com/cenkalti/backoff/v4"

	"time"
)

type BackOff struct {
	MaxRetry        uint64
	MaxInterval     time.Duration
	MaxElapsedTime  time.Duration
	InitialInterval time.Duration
}

func NewBackoff(maxRetry uint64, maxInterval, maxElapsedTime, initialInterval time.Duration) *BackOff {
	return &BackOff{
		MaxRetry:        maxRetry,
		MaxInterval:     maxInterval,
		MaxElapsedTime:  maxElapsedTime,
		InitialInterval: initialInterval,
	}
}

func (b *BackOff) Retry(ctx context.Context, operation backoff.Operation) error {
	op := backoff.Operation(operation)
	notify := func(err error, backoffDuration time.Duration) {
		slog.Debug("retrying", "err", err, "duration", backoffDuration)
	}
	if err := backoff.RetryNotify(op, b.getBackoff(), notify); err != nil {
		return err
	}
	return nil

}
func (b *BackOff) getBackoff() backoff.BackOff {
	requestExpBackOff := backoff.NewExponentialBackOff()
	requestExpBackOff.InitialInterval = b.InitialInterval
	requestExpBackOff.RandomizationFactor = 0.5
	requestExpBackOff.Multiplier = 1.5
	requestExpBackOff.MaxInterval = b.MaxInterval
	requestExpBackOff.MaxElapsedTime = b.MaxElapsedTime
	return backoff.WithMaxRetries(requestExpBackOff, b.MaxRetry)
}
