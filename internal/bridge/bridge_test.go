package bridge

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"
)

// pipeConn wraps net.Conn to satisfy io.ReadWriteCloser for Bridge.
type pipeConn struct{ net.Conn }

func TestBridge_ForwardsBytes(t *testing.T) {
	// a <-> b are two ends of an in-process pipe pair.
	a1, a2 := net.Pipe()
	b1, b2 := net.Pipe()

	ctx := context.Background()

	go Bridge(ctx, a2, b2)

	msg := []byte("hello from a to b")
	_, err := a1.Write(msg)
	if err != nil {
		t.Fatalf("write to a1: %v", err)
	}

	buf := make([]byte, len(msg))
	_, err = io.ReadFull(b1, buf)
	if err != nil {
		t.Fatalf("read from b1: %v", err)
	}
	if !bytes.Equal(buf, msg) {
		t.Fatalf("got %q, want %q", buf, msg)
	}

	reply := []byte("hello from b to a")
	_, err = b1.Write(reply)
	if err != nil {
		t.Fatalf("write to b1: %v", err)
	}

	buf2 := make([]byte, len(reply))
	_, err = io.ReadFull(a1, buf2)
	if err != nil {
		t.Fatalf("read from a1: %v", err)
	}
	if !bytes.Equal(buf2, reply) {
		t.Fatalf("got %q, want %q", buf2, reply)
	}

	a1.Close()
	b1.Close()
}

func TestBridge_CloseOnEOF(t *testing.T) {
	a1, a2 := net.Pipe()
	b1, b2 := net.Pipe()

	done := make(chan Result, 1)
	ctx := context.Background()
	go func() {
		done <- Bridge(ctx, a2, b2)
	}()

	// Close a1 — EOF propagates to a2, bridge should close b2, unblocking b1.
	a1.Close()

	// b1 should receive EOF promptly.
	b1.SetDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 4)
	_, err := b1.Read(buf)
	if err == nil {
		t.Fatal("expected EOF on b1 after a1 closed, got nil")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Bridge did not return after both sides closed")
	}

	b1.Close()
}

func TestBridge_ContextCancel(t *testing.T) {
	a1, a2 := net.Pipe()
	b1, b2 := net.Pipe()

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan Result, 1)
	go func() {
		done <- Bridge(ctx, a2, b2)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Bridge did not return after context cancel")
	}

	a1.Close()
	b1.Close()
}

func TestBridge_ByteCounts(t *testing.T) {
	a1, a2 := net.Pipe()
	b1, b2 := net.Pipe()

	done := make(chan Result, 1)
	ctx := context.Background()
	go func() {
		done <- Bridge(ctx, a2, b2)
	}()

	aMsg := []byte("aaa") // 3 bytes a→b
	bMsg := []byte("bb")  // 2 bytes b→a

	a1.Write(aMsg)
	buf := make([]byte, len(aMsg))
	io.ReadFull(b1, buf)

	b1.Write(bMsg)
	buf2 := make([]byte, len(bMsg))
	io.ReadFull(a1, buf2)

	a1.Close()
	b1.Close()

	select {
	case res := <-done:
		if res.BytesAtoB != int64(len(aMsg)) {
			t.Errorf("BytesAtoB = %d, want %d", res.BytesAtoB, len(aMsg))
		}
		if res.BytesBtoA != int64(len(bMsg)) {
			t.Errorf("BytesBtoA = %d, want %d", res.BytesBtoA, len(bMsg))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Bridge did not return")
	}
}
