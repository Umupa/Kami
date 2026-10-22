package macos

import (
	"context"
	"sync"
)

// awaitNative transfers one callback result to its caller. A result arriving
// after cancellation (including a result racing the deadline) is disposed.
// start must initiate an asynchronous operation and return without waiting.
func awaitNative[T any](ctx context.Context, start func(func(T)), discard func(T)) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	done := make(chan T, 1)
	var mu sync.Mutex
	abandoned, delivered := false, false
	start(func(v T) {
		mu.Lock()
		if abandoned || delivered {
			mu.Unlock()
			discard(v)
			return
		}
		delivered = true
		done <- v
		mu.Unlock()
	})
	select {
	case v := <-done:
		return v, nil
	case <-ctx.Done():
		mu.Lock()
		abandoned = true
		if delivered {
			v := <-done
			mu.Unlock()
			discard(v)
		} else {
			mu.Unlock()
		}
		return zero, ctx.Err()
	}
}
