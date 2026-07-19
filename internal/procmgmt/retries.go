package procmgmt

import (
	"context"
	"fmt"
	"time"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

// Retry defaults mirroring Invoke-ADTCommandWithRetries ($Retries = 3,
// $SleepDuration = 5s).
const (
	DefaultRetryAttempts = 3
	DefaultRetryDelay    = 5 * time.Second
)

// Retry ports Invoke-ADTCommandWithRetries: it invokes fn and, on any error,
// waits delay and tries again, up to retries additional attempts (so fn runs
// at most retries+1 times, exactly like the PowerShell `$i -ge $Retries`
// loop). Non-positive retries/delay fall back to the PSADT defaults. The
// last error is returned verbatim when attempts are exhausted; context
// cancellation aborts immediately, including mid-sleep.
func Retry(
	ctx context.Context,
	retries int,
	delay time.Duration,
	fn func(context.Context) error,
) error {
	if fn == nil {
		return fmt.Errorf("procmgmt: Retry requires a function: %w", winerr.ErrInvalidOption)
	}
	if retries <= 0 {
		retries = DefaultRetryAttempts
	}
	if delay <= 0 {
		delay = DefaultRetryDelay
	}
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("procmgmt: retry aborted: %w", err)
		}
		err := fn(ctx)
		if err == nil {
			return nil
		}
		if attempt >= retries {
			return err
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("procmgmt: retry aborted: %w", ctx.Err())
		case <-timer.C:
		}
	}
}
