package keeper

import (
	"context"
	"sync"
	"testing"
	"time"

	wasmvm "github.com/sei-protocol/sei-chain/sei-wasmvm"
	"github.com/stretchr/testify/require"
)

type callerStore struct {
	wasmvm.KVStore
	ctx context.Context
}

func (s callerStore) Context() context.Context { return s.ctx }

func TestVMLockDeadline(t *testing.T) {
	w := &VMWrapper{mu: &sync.Mutex{}}
	w.mu.Lock()
	defer w.mu.Unlock()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.lockStore(callerStore{ctx: ctx}) }()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(time.Second):
		t.Fatal("expired caller stays in the VM lock queue")
	}
}

func TestVMLockCancelledBeforeAcquire(t *testing.T) {
	w := &VMWrapper{mu: &sync.Mutex{}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, w.lockStore(callerStore{ctx: ctx}), context.Canceled)
	require.True(t, w.mu.TryLock())
	w.mu.Unlock()
}

func TestVMLockAfterRelease(t *testing.T) {
	w := &VMWrapper{mu: &sync.Mutex{}}
	w.mu.Lock()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.lockStore(callerStore{ctx: ctx}) }()
	w.mu.Unlock()
	require.NoError(t, <-done)
	require.False(t, w.mu.TryLock())
	w.mu.Unlock()
}

func TestVMLockBlockExecution(t *testing.T) {
	w := &VMWrapper{mu: &sync.Mutex{}}
	w.mu.Lock()
	done := make(chan error, 1)
	go func() { done <- w.lockStore(callerStore{ctx: context.Background()}) }()
	select {
	case <-done:
		t.Fatal("block execution bypasses the VM lock")
	case <-time.After(20 * time.Millisecond):
	}
	w.mu.Unlock()
	require.NoError(t, <-done)
	w.mu.Unlock()
}
