//go:build !windows

package cmd

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"
)

// TestInterruptibleCancelsOnSignal checks that Ctrl-C reaches the rewrite as a
// canceled context, which is what lets the engine roll the run back.
func TestInterruptibleCancelsOnSignal(t *testing.T) {
	ctx, stop := interruptible(context.Background())
	defer stop()
	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("kill: %v", err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the context was not canceled by SIGINT")
	}
}
