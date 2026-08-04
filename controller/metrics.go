package controller

import (
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// GetMetrics returns Prometheus-format metrics for monitoring systems.
// Exposed at GET /api/metrics (admin-only).
func GetMetrics(c *gin.Context) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	status := common.GetSystemStatus()
	httpStats := middleware.GetStats()
	queueStats := middleware.GetQueueStats()
	activeBreakers := service.GetOpenCircuitBreakerCount()
	inFlight := model.GetAllInFlightRequests()

	var totalInFlight int64
	for _, v := range inFlight {
		totalInFlight += v
	}

	// Build metrics in Prometheus text format.
	lines := []string{
		"# HELP new_api_build_info Build information",
		"# TYPE new_api_build_info gauge",
		fmt.Sprintf("new_api_build_info{version=\"%s\"} 1", common.Version),
		"",
		"# HELP new_api_uptime_seconds Server uptime in seconds",
		"# TYPE new_api_uptime_seconds gauge",
		fmt.Sprintf("new_api_uptime_seconds %d", int(time.Now().Unix() - common.StartTime)),
		"",
		"# HELP new_api_cpu_usage_percent CPU usage percentage",
		"# TYPE new_api_cpu_usage_percent gauge",
		fmt.Sprintf("new_api_cpu_usage_percent %.1f", status.CPUUsage),
		"",
		"# HELP new_api_memory_usage_percent Memory usage percentage",
		"# TYPE new_api_memory_usage_percent gauge",
		fmt.Sprintf("new_api_memory_usage_percent %.1f", status.MemoryUsage),
		"",
		"# HELP new_api_disk_usage_percent Disk usage percentage",
		"# TYPE new_api_disk_usage_percent gauge",
		fmt.Sprintf("new_api_disk_usage_percent %.1f", status.DiskUsage),
		"",
		"# HELP new_api_active_connections Current active HTTP connections",
		"# TYPE new_api_active_connections gauge",
		fmt.Sprintf("new_api_active_connections %d", httpStats.ActiveConnections),
		"",
		"# HELP new_api_active_requests Current in-flight relay requests",
		"# TYPE new_api_active_requests gauge",
		fmt.Sprintf("new_api_active_requests %d", totalInFlight),
		"",
		"# HELP new_api_goroutines Number of goroutines",
		"# TYPE new_api_goroutines gauge",
		fmt.Sprintf("new_api_goroutines %d", runtime.NumGoroutine()),
		"",
		"# HELP new_api_queue_size Current request queue size",
		"# TYPE new_api_queue_size gauge",
		fmt.Sprintf("new_api_queue_size %d", queueStats["queue_size"]),
		"",
		"# HELP new_api_open_circuit_breakers Number of open circuit breakers",
		"# TYPE new_api_open_circuit_breakers gauge",
		fmt.Sprintf("new_api_open_circuit_breakers %d", activeBreakers),
		"",
		"# HELP new_api_go_mem_alloc_bytes Go memory alloc bytes",
		"# TYPE new_api_go_mem_alloc_bytes gauge",
		fmt.Sprintf("new_api_go_mem_alloc_bytes %d", memStats.Alloc),
		"",
		"# HELP new_api_go_gc_count Number of GC cycles completed",
		"# TYPE new_api_go_gc_count counter",
		fmt.Sprintf("new_api_go_gc_count %d", memStats.NumGC),
		"",
	}

	for _, line := range lines {
		c.Writer.WriteString(line + "\n")
	}
	c.Writer.WriteHeader(http.StatusOK)
	c.Header("Content-Type", "text/plain; charset=utf-8")
}