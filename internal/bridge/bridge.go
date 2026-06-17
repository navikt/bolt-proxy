package bridge

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
)

// Result holds the byte counts from a completed bridge session.
type Result struct {
	BytesAtoB int64
	BytesBtoA int64
}

// Bridge pumps bytes bidirectionally between a and b until either side closes
// or the context is cancelled. Both sides are closed before Bridge returns.
//
// No read/write deadlines are set on the connections — the session lives as
// long as the underlying protocol (Bolt) keeps it open. Callers should not
// wrap connections with timeouts that would kill long-running queries.
func Bridge(ctx context.Context, a, b io.ReadWriteCloser) Result {
	var bytesAtoB, bytesBtoA atomic.Int64

	// cancelOnce ensures we only close both sides once.
	var closeOnce sync.Once
	closeAll := func() {
		closeOnce.Do(func() {
			a.Close()
			b.Close()
		})
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		n, _ := io.Copy(b, a)
		bytesAtoB.Store(n)
		closeAll()
	}()

	go func() {
		defer wg.Done()
		n, _ := io.Copy(a, b)
		bytesBtoA.Store(n)
		closeAll()
	}()

	// If context is cancelled, tear down both sides so the goroutines unblock.
	go func() {
		<-ctx.Done()
		closeAll()
	}()

	wg.Wait()
	return Result{
		BytesAtoB: bytesAtoB.Load(),
		BytesBtoA: bytesBtoA.Load(),
	}
}
