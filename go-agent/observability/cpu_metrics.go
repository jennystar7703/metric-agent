// In go-agent/observability/cpu_metrics.go

//go:build !gpu

package observability

import (
	"go.opentelemetry.io/otel/metric"
)

// This is the FAKE implementation for CPU-only builds.
// It does nothing and ensures no NVML code is included.
func observeGpuMetrics(o metric.Observer, gpuGauge, gpuTempGauge, gpuVRAMGauge metric.Float64ObservableGauge) {
	// Intentionally do nothing.
}