package nrfepd

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// The scan that will not stop is the whole reason this exists.
//
// One HTTP client hung up in the middle of GET /v1/devices and the service was
// wedged until it was restarted: every later request answered device-busy,
// because the handler releases the adapter after the scan returns and the scan
// never did. StopScan had been called before Scan began, which does nothing,
// and there was nobody left to call it again.
func TestAScanIsAskedAgainWhenTheFirstStopArrivesTooEarly(t *testing.T) {
	done := make(chan error, 1)
	var stops atomic.Int32
	// The first stop lands before the scan is running and is lost, exactly as
	// the real one is. The scan answers only once it has been asked again.
	stop := func() error {
		if stops.Add(1) >= 2 {
			done <- nil
		}
		return nil
	}
	if err := stopScanning(stop, done); err != nil {
		t.Fatalf("gave up after %d attempts: %v", stops.Load(), err)
	}
	if got := stops.Load(); got < 2 {
		t.Errorf("asked %d times; the point is that once is not enough", got)
	}
}

// A scan already running stops on the first ask, and nothing is retried.
func TestAScanThatStopsAtOnceIsNotAskedTwice(t *testing.T) {
	done := make(chan error, 1)
	var stops atomic.Int32
	stop := func() error { stops.Add(1); done <- nil; return nil }
	if err := stopScanning(stop, done); err != nil {
		t.Fatal(err)
	}
	if got := stops.Load(); got != 1 {
		t.Errorf("asked %d times, want 1", got)
	}
}

// Whatever the scan reports on the way out is what the caller hears.
func TestTheScansOwnFailureIsPassedOn(t *testing.T) {
	done := make(chan error, 1)
	refused := errors.New("the adapter said no")
	if err := stopScanning(func() error { done <- refused; return nil }, done); !errors.Is(err, refused) {
		t.Errorf("reported %v, want %v", err, refused)
	}
}

// A scan that never answers has to be given up on. Blocking here is what the
// service did before, and a wedged adapter cannot be recovered from; an error
// can, because the caller goes on to release the adapter and say what happened.
func TestAScanThatNeverAnswersIsGivenUpOn(t *testing.T) {
	if testing.Short() {
		t.Skip("this one waits out the limit")
	}
	done := make(chan error) // nothing is ever sent
	start := time.Now()
	err := stopScanning(func() error { return nil }, done)
	if err == nil {
		t.Fatal("waited for a scan that never answers and called it success")
	}
	if waited := time.Since(start); waited > stopScanLimit+2*time.Second {
		t.Errorf("waited %s, which is not a bound", waited)
	}
}
