package middleware

import (
	"container/heap"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestPriority defines the priority level of a request.
type RequestPriority int

const (
	PriorityLow    RequestPriority = 0
	PriorityNormal RequestPriority = 1
	PriorityHigh   RequestPriority = 2
	PriorityCritical RequestPriority = 3
)

// queuedRequest represents a request waiting in the queue.
type queuedRequest struct {
	priority int
	enqueued time.Time
	ctx      *gin.Context
	done     chan struct{}
	index    int // index in the heap
}

// priorityQueue implements heap.Interface for queued requests.
type priorityQueue []*queuedRequest

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	// Higher priority first, then FIFO for same priority.
	if pq[i].priority != pq[j].priority {
		return pq[i].priority > pq[j].priority
	}
	return pq[i].enqueued.Before(pq[j].enqueued)
}

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *priorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*queuedRequest)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*pq = old[0 : n-1]
	return item
}

// RequestQueueConfig holds configuration for the request queue.
type RequestQueueConfig struct {
	Enabled          bool
	MaxQueueSize     int
	MaxConcurrency   int32
	QueueTimeout     time.Duration
}

var (
	requestQueueConfig RequestQueueConfig
	requestQueueMu     sync.RWMutex
	requestQueue       priorityQueue
	requestQueueLock   sync.Mutex
	activeRequests     int32
	queueCond          = sync.NewCond(&requestQueueLock)
)

// GetRequestQueueConfig returns the current request queue configuration.
func GetRequestQueueConfig() RequestQueueConfig {
	requestQueueMu.RLock()
	defer requestQueueMu.RUnlock()
	return requestQueueConfig
}

// SetRequestQueueConfig updates the request queue configuration.
func SetRequestQueueConfig(cfg RequestQueueConfig) {
	requestQueueMu.Lock()
	requestQueueConfig = cfg
	requestQueueMu.Unlock()
}

// GetActiveRequestCount returns the number of currently active requests.
func GetActiveRequestCount() int32 {
	return atomic.LoadInt32(&activeRequests)
}

// RequestQueueMiddleware returns a Gin middleware that queues requests when
// the system is under heavy load, applying backpressure.
func RequestQueueMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := GetRequestQueueConfig()
		if !cfg.Enabled || cfg.MaxConcurrency <= 0 {
			c.Next()
			return
		}

		// Try to acquire a slot immediately.
		current := atomic.LoadInt32(&activeRequests)
		if current < cfg.MaxConcurrency {
			if atomic.CompareAndSwapInt32(&activeRequests, current, current+1) {
				defer atomic.AddInt32(&activeRequests, -1)
				c.Next()
				return
			}
		}

		// Queue the request.
		req := &queuedRequest{
			priority: int(PriorityNormal),
			enqueued: time.Now(),
			ctx:      c,
			done:     make(chan struct{}),
		}

		// Determine priority from context.
		if p, ok := c.Get("request_priority"); ok {
			if pri, ok := p.(RequestPriority); ok {
				req.priority = int(pri)
			}
		}

		requestQueueLock.Lock()
		if len(requestQueue) >= cfg.MaxQueueSize {
			requestQueueLock.Unlock()
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": gin.H{
					"message": "Server is busy, please try again later",
					"type":    "server_overloaded",
					"code":    "server_overloaded",
				},
			})
			return
		}
		heap.Push(&requestQueue, req)
		requestQueueLock.Unlock()

		// Wait for our turn or timeout.
		select {
		case <-req.done:
			defer atomic.AddInt32(&activeRequests, -1)
			c.Next()
		case <-time.After(cfg.QueueTimeout):
			// Remove from queue.
			requestQueueLock.Lock()
			if req.index >= 0 && req.index < len(requestQueue) && requestQueue[req.index] == req {
				heap.Remove(&requestQueue, req.index)
			}
			requestQueueLock.Unlock()
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": gin.H{
					"message": "Request timed out in queue",
					"type":    "server_overloaded",
					"code":    "request_timeout",
				},
			})
		}
	}
}

// StartRequestQueueDispatching starts the background goroutine that dispatches
// queued requests as slots become available.
func StartRequestQueueDispatching() {
	go func() {
		for {
			cfg := GetRequestQueueConfig()
			if !cfg.Enabled || cfg.MaxConcurrency <= 0 {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			requestQueueLock.Lock()
			for len(requestQueue) > 0 {
				current := atomic.LoadInt32(&activeRequests)
				if current >= cfg.MaxConcurrency {
					break
				}
				if atomic.CompareAndSwapInt32(&activeRequests, current, current+1) {
					req := heap.Pop(&requestQueue).(*queuedRequest)
					// Signal the waiting goroutine.
					close(req.done)
				} else {
					break
				}
			}
			requestQueueLock.Unlock()

			time.Sleep(10 * time.Millisecond)
		}
	}()
}

// GetQueueStats returns statistics about the current request queue.
func GetQueueStats() map[string]interface{} {
	requestQueueLock.Lock()
	defer requestQueueLock.Unlock()

	queueSize := len(requestQueue)
	avgWaitMs := int64(0)
	if queueSize > 0 {
		now := time.Now()
		totalWait := int64(0)
		for _, r := range requestQueue {
			totalWait += now.Sub(r.enqueued).Milliseconds()
		}
		avgWaitMs = totalWait / int64(queueSize)
	}

	return map[string]interface{}{
		"active_requests":  atomic.LoadInt32(&activeRequests),
		"queue_size":       queueSize,
		"max_queue_size":   GetRequestQueueConfig().MaxQueueSize,
		"max_concurrency":  GetRequestQueueConfig().MaxConcurrency,
		"avg_wait_ms":      avgWaitMs,
	}
}