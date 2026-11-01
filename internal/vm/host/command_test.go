package host

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestLockSerializesGoroutinesAndHonorsCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lifecycle.lock")
	release, err := Lock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	waiting := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		close(waiting)
		unlock, err := Lock(ctx, path)
		if err == nil {
			unlock()
		}
		result <- err
	}()
	<-waiting
	cancel()
	err = <-result
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	release()
	ctx, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	unlock, err := Lock(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	if err != nil {
		t.Fatal("lock must be available after release")
	}
}
