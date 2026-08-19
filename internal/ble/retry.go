package ble

import (
	"context"
	"time"
)

// Retry runs an attempt more than once, because a tag that is merely between
// advertising windows looks exactly like a tag that is not there. Only a second
// look tells them apart, and on hardware the wait is an unsteady 6s to 18s
// after a disconnect.
type Retry struct {
	Attempts int
	Delay    time.Duration
	Logf     func(string, ...any)
}

// Do runs attempt up to Attempts times, waiting Delay between them and
// returning the last error. what names the operation in the log so an identify
// that failed reads apart from a write that did.
func (r Retry) Do(ctx context.Context, what string, attempt func() error) error {
	attempts := r.Attempts
	if attempts <= 0 {
		attempts = 1
	}
	var lastErr error
	for number := 1; number <= attempts; number++ {
		if err := attempt(); err == nil {
			return nil
		} else {
			lastErr = err
			r.logf("%sattempt %d/%d failed: %v", label(what), number, attempts, err)
		}
		if number < attempts {
			if err := Wait(ctx, r.Delay); err != nil {
				return err
			}
		}
	}
	return lastErr
}

// label keeps an unnamed retry reading as "attempt 1/3" rather than leaving a
// space where a name would have been.
func label(what string) string {
	if what == "" {
		return ""
	}
	return what + " "
}

func (r Retry) logf(format string, args ...any) {
	if r.Logf != nil {
		r.Logf(format, args...)
	}
}

// Wait sleeps unless the context ends first. A zero or negative duration
// returns at once without consulting the context, which is what a caller that
// configured no delay asked for.
func Wait(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
