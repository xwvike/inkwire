package nrfepd

import (
	"context"
	"errors"
	"testing"
	"time"
)

// slowestReappearance is how long the tag took to advertise again after a mode
// change disconnected it, measured on 2026-08-17 over three runs: 6s, 18s and
// 12s. The spread is the point rather than the maximum — a 15s scan lands on
// either side of it depending on the run, so whether a single attempt finds
// the tag is close to a coin toss.
const slowestReappearance = 18 * time.Second

// One attempt cannot decide that a tag is absent, because the scan it decides
// on is shorter than the tag's own silence. The budget across attempts has to
// outlast that silence.
func TestTheRetryBudgetOutlastsATagsSilence(t *testing.T) {
	if DefaultScanTimeout > slowestReappearance {
		t.Skip("a single scan now covers the measured silence; this test guards the case where it does not")
	}
	budget := time.Duration(DefaultAttempts)*DefaultScanTimeout +
		time.Duration(DefaultAttempts-1)*DefaultRetryDelay
	if budget <= slowestReappearance {
		t.Errorf("%d attempts of %s with %s between them allow %s, "+
			"which does not outlast the %s a tag was measured to stay silent",
			DefaultAttempts, DefaultScanTimeout, DefaultRetryDelay, budget, slowestReappearance)
	}
	if DefaultAttempts < 2 {
		t.Errorf("attempts = %d leaves no second look for a scan that misses the advertising window", DefaultAttempts)
	}
}

// retrying is what separates an entry point that survives a missed advertising
// window from one that does not, so it is worth knowing it actually retries.
func TestRetryingKeepsTryingUntilOneAttemptSucceeds(t *testing.T) {
	driver := &Driver{Attempts: 3, RetryDelay: time.Millisecond}
	calls := 0
	err := driver.retrying(context.Background(), func() error {
		calls++
		if calls < 3 {
			return errors.New("no tag found")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("gave up with %v after %d attempts", err, calls)
	}
	if calls != 3 {
		t.Errorf("made %d attempts, want 3", calls)
	}
}

// The last failure is what the caller is told, not a message invented here.
func TestRetryingReportsTheLastFailure(t *testing.T) {
	driver := &Driver{Attempts: 2, RetryDelay: time.Millisecond}
	last := errors.New("the second one")
	err := driver.retrying(context.Background(), func() error { return last })
	if !errors.Is(err, last) {
		t.Errorf("reported %v, want %v", err, last)
	}
}

// A cancelled context stops the waiting rather than sleeping out the delay.
func TestRetryingStopsWhenTheContextDoes(t *testing.T) {
	driver := &Driver{Attempts: 4, RetryDelay: time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	start := time.Now()
	err := driver.retrying(ctx, func() error {
		calls++
		cancel()
		return errors.New("no tag found")
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("reported %v, want the cancellation", err)
	}
	if calls != 1 {
		t.Errorf("made %d attempts after cancellation, want 1", calls)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("waited %s before noticing the cancellation", elapsed)
	}
}

// SetModeWithRetry reads the clock per attempt. A retry that reused the first
// attempt's reading would set the tag to a time that had already passed, by
// however long the failures took — and the failures here are scans, so that is
// tens of seconds rather than milliseconds.
func TestEachModeAttemptReadsTheClockAgain(t *testing.T) {
	driver := &Driver{Attempts: 3, RetryDelay: time.Millisecond, Target: "nothing"}
	var asked []time.Time
	base := time.Date(2026, 8, 17, 1, 45, 0, 0, time.UTC)
	clock := func() time.Time {
		now := base.Add(time.Duration(len(asked)) * 20 * time.Second)
		asked = append(asked, now)
		return now
	}
	// The adapter is nil, so every attempt fails before touching a radio; what
	// is under test is how often the clock is read, not what reaches the tag.
	_ = driver.SetModeWithRetry(context.Background(), clock, ModePicture, nil)

	if len(asked) != 3 {
		t.Fatalf("read the clock %d times over 3 attempts", len(asked))
	}
	for index := 1; index < len(asked); index++ {
		if !asked[index].After(asked[index-1]) {
			t.Errorf("attempt %d reused the time from attempt %d", index+1, index)
		}
	}
}
