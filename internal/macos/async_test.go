package macos

import (
	"context"
	"errors"
	"testing"
)

func TestAbandonedNativeResultIsDisposed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var complete func(int)
	discarded := 0
	_, err := awaitNative(ctx, func(deliver func(int)) { complete = deliver; cancel() }, func(v int) { discarded += v })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error: %v", err)
	}
	complete(7)
	if discarded != 7 {
		t.Fatalf("late result disposal: %d", discarded)
	}
}

func TestNativeResultDeliveredOnce(t *testing.T) {
	var complete func(int)
	discarded := 0
	got, err := awaitNative(context.Background(), func(deliver func(int)) { complete = deliver; deliver(4) }, func(v int) { discarded += v })
	if err != nil || got != 4 {
		t.Fatalf("result: %d %v", got, err)
	}
	complete(5)
	if discarded != 5 {
		t.Fatal("duplicate result was not disposed")
	}
}

func TestCanceledNativeOperationDoesNotStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := awaitNative(ctx, func(func(int)) { t.Fatal("canceled operation started") }, func(int) {})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error: %v", err)
	}
}

func TestNativeDeadlineRaceDisposesExactlyOnce(t *testing.T) {
	for range 100 {
		ctx, cancel := context.WithCancel(context.Background())
		disposed := 0
		dispose := func(int) { disposed++ }
		value, err := awaitNative(ctx, func(deliver func(int)) { deliver(1); cancel() }, dispose)
		if err == nil {
			dispose(value)
		} else if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		if disposed != 1 {
			t.Fatalf("result was disposed %d times", disposed)
		}
	}
}
