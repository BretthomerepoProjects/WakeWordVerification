//go:build spot && linux

package monitoring

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestSupervisorRestartsUnexpectedExitAndStopsOnDisable(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var enabled atomic.Bool
	enabled.Store(true)
	started := make(chan struct{}, 2)
	stopped := make(chan struct{})
	finished := make(chan struct{})
	svc := &service{}
	go func() {
		defer close(finished)
		var starts int
		supervise(ctx, svc, enabled.Load, func(value bool) bool { return value },
			func(ctx context.Context, _ bool, state func(string)) error {
				starts++
				started <- struct{}{}
				if starts == 1 {
					return errors.New("unexpected exit")
				}
				state("Running")
				<-ctx.Done()
				close(stopped)
				return nil
			})
	}()
	wait := func(signal <-chan struct{}, label string) {
		t.Helper()
		select {
		case <-signal:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for %s", label)
		}
	}
	wait(started, "initial start")
	wait(started, "restart")
	enabled.Store(false)
	wait(stopped, "intentional stop")
	select {
	case <-started:
		t.Fatal("disabled service started again")
	case <-time.After(1100 * time.Millisecond):
	}
	if got := svc.get(); got != "Disabled" {
		t.Fatalf("status = %q, want Disabled", got)
	}
	cancel()
	wait(finished, "supervisor shutdown")
}
