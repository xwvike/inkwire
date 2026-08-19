package gicisky

import (
	"testing"
	"time"
)

// These timings were measured against the tag on 2026-08-13, and the margins
// between them are the only reason a healthy write is not cut short. The test
// exists so that shortening a timeout has to argue with the measurement.
func TestTimeoutsClearTheMeasuredDevice(t *testing.T) {
	const (
		slowestScan       = 11530 * time.Millisecond
		slowestConnect    = 7790 * time.Millisecond
		slowestResponse   = 110 * time.Millisecond
		slowestHealthyRun = 20521 * time.Millisecond
	)

	if DefaultScanTimeout <= slowestScan {
		t.Errorf("scan timeout %s does not clear the slowest measured scan %s", DefaultScanTimeout, slowestScan)
	}
	// The response timeout also has to cover the first exchange, which the
	// tag only answers once it has connected and discovered services.
	if DefaultResponseTimeout <= slowestResponse {
		t.Errorf("response timeout %s does not clear the slowest measured reply %s", DefaultResponseTimeout, slowestResponse)
	}
	// One attempt must be able to complete a whole healthy write, or every
	// push would burn its retries on a tag that was answering all along.
	budget := DefaultScanTimeout + DefaultNotifyReadyDelay + slowestConnect
	if budget <= slowestHealthyRun {
		t.Errorf("one attempt allows %s, less than the slowest healthy write %s", budget, slowestHealthyRun)
	}
	if DefaultAttempts < 2 {
		t.Errorf("attempts = %d leaves no retry for a scan that misses the advertising window", DefaultAttempts)
	}
}
