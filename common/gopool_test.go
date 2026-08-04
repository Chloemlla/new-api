package common

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBoundedGoPoolCapsConcurrency(t *testing.T) {
	const workers = 2
	p := newBoundedGoPool(workers, workers, relayGoPoolPanic)
	defer p.Close()

	started := make(chan struct{}, workers*2+1)
	release := make(chan struct{})

	var mu sync.Mutex
	var running, maxRunning int

	task := func() {
		mu.Lock()
		running++
		if running > maxRunning {
			maxRunning = running
		}
		mu.Unlock()
		started <- struct{}{}
		<-release
		mu.Lock()
		running--
		mu.Unlock()
	}

	// Occupy both workers.
	p.Go(task)
	p.Go(task)
	for i := 0; i < workers; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			require.FailNow(t, "worker task did not start")
		}
	}

	// Fill the queue while the workers are busy.
	p.Go(task)
	p.Go(task)

	// A further submit must block until a worker frees a slot.
	submitted := make(chan struct{})
	go func() {
		p.Go(task)
		close(submitted)
	}()
	select {
	case <-submitted:
		require.FailNow(t, "submit returned while the pool was saturated")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)

	// The queued tasks and the previously blocked submit (workers+1 of them)
	// eventually run.
	for i := 0; i < workers+1; i++ {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			require.FailNow(t, "task did not start after workers were freed")
		}
	}

	mu.Lock()
	defer mu.Unlock()
	assert.LessOrEqual(t, maxRunning, workers)
}

func TestBoundedGoPoolRecoversPanic(t *testing.T) {
	p := newBoundedGoPool(1, 1, relayGoPoolPanic)
	defer p.Close()

	// A panicking task must not kill the worker: the panic handler still sees
	// the task context (here the stream stop channel).
	stopChan := make(chan bool, 1)
	ctx := context.WithValue(context.Background(), "stop_chan", stopChan)

	p.CtxGo(ctx, func() { panic("boom") })

	select {
	case <-stopChan:
	case <-time.After(time.Second):
		require.FailNow(t, "panic handler did not signal the stop channel")
	}

	ran := make(chan struct{})
	p.Go(func() { close(ran) })
	select {
	case <-ran:
	case <-time.After(time.Second):
		require.FailNow(t, "pool stopped processing tasks after a panic")
	}
}

func TestBoundedGoPoolSkipsTaskWithCanceledContext(t *testing.T) {
	p := newBoundedGoPool(1, 1, relayGoPoolPanic)
	defer p.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ran := make(chan struct{})
	p.CtxGo(ctx, func() { close(ran) })

	select {
	case <-ran:
		require.FailNow(t, "task ran after its context was canceled")
	case <-time.After(100 * time.Millisecond):
	}
}