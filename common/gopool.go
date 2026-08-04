package common

import (
	"context"
	"fmt"
	"sync"
)

const (
	defaultRelayGoPoolWorkers = 256
	defaultRelayGoPoolQueue   = 1024
)

var (
	relayGoPool     *boundedGoPool
	relayGoPoolOnce sync.Once
)

// boundedGoPool executes submitted tasks on a fixed set of worker goroutines
// with a bounded pending queue. Both the number of workers and the queue
// capacity are fixed at construction, so a burst of submissions can neither
// spawn an unbounded number of goroutines nor grow the wait queue without
// limit: once every worker is busy and the queue is full, CtxGo blocks the
// caller until a slot frees (backpressure) or the caller's context is
// canceled.
type boundedGoPool struct {
	queue        chan poolTask
	panicHandler func(context.Context, interface{})
	wg           sync.WaitGroup
}

// poolTask pairs the function to run with the context it was submitted with.
// The context is forwarded to the panic handler if f panics.
type poolTask struct {
	ctx context.Context
	f   func()
}

func newBoundedGoPool(workers, queueSize int, panicHandler func(context.Context, interface{})) *boundedGoPool {
	if workers < 1 {
		workers = 1
	}
	if queueSize < 1 {
		queueSize = 1
	}
	p := &boundedGoPool{
		queue:        make(chan poolTask, queueSize),
		panicHandler: panicHandler,
	}
	for i := 0; i < workers; i++ {
		p.wg.Add(1)
		go p.worker()
	}
	return p
}

// worker drains the task queue until the queue is closed by Close.
func (p *boundedGoPool) worker() {
	defer p.wg.Done()
	for t := range p.queue {
		func() {
			defer func() {
				if r := recover(); r != nil && p.panicHandler != nil {
					p.panicHandler(t.ctx, r)
				}
			}()
			t.f()
		}()
	}
}

// Go schedules f on the pool. See CtxGo for the saturation behavior.
func (p *boundedGoPool) Go(f func()) {
	p.CtxGo(context.Background(), f)
}

// CtxGo schedules f on the pool. f is skipped (not executed) when ctx is
// already done. When the pool is saturated, CtxGo blocks until a worker frees
// a slot, or until ctx is canceled.
func (p *boundedGoPool) CtxGo(ctx context.Context, f func()) {
	if ctx.Err() != nil {
		return
	}
	t := poolTask{ctx: ctx, f: f}
	select {
	case p.queue <- t:
	case <-ctx.Done():
	}
}

// Close stops the workers after the pending queue drains and waits for them to
// exit. It must not be called while tasks that never return are still running.
func (p *boundedGoPool) Close() {
	close(p.queue)
	p.wg.Wait()
}

// relayGoPoolPanic is the panic handler for tasks submitted through
// RelayCtxGo. It mirrors the previous gopool.RelayPool handler: signal the
// stream stop channel captured in the task context (if any) and record the
// panic in the system log.
func relayGoPoolPanic(ctx context.Context, i interface{}) {
	if stopChan, ok := ctx.Value("stop_chan").(chan bool); ok {
		SafeSendBool(stopChan, true)
	}
	SysError(fmt.Sprintf("panic in gopool.RelayPool: %v", i))
}

// RelayCtxGo schedules f on the bounded relay goroutine pool. See
// boundedGoPool.CtxGo for the saturation and cancellation behavior. The pool
// size is configurable via RELAY_GO_POOL_WORKERS and RELAY_GO_POOL_QUEUE_SIZE.
func RelayCtxGo(ctx context.Context, f func()) {
	relayGoPoolOnce.Do(func() {
		relayGoPool = newBoundedGoPool(
			GetEnvOrDefault("RELAY_GO_POOL_WORKERS", defaultRelayGoPoolWorkers),
			GetEnvOrDefault("RELAY_GO_POOL_QUEUE_SIZE", defaultRelayGoPoolQueue),
			relayGoPoolPanic,
		)
	})
	relayGoPool.CtxGo(ctx, f)
}