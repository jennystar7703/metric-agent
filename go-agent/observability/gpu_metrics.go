// In go-agent/observability/gpu_metrics.go

//go:build gpu

package observability

import (
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// This is the REAL implementation that collects GPU metrics.
// It will only be compiled when the 'gpu' build tag is used.
func observeGpuMetrics(o metric.Observer, gpuGauge, gpuTempGauge, gpuVRAMGauge metric.Float64ObservableGauge) {
	if count, ret := nvml.DeviceGetCount(); ret == nvml.SUCCESS {
		for i := 0; i < int(count); i++ {
			dev, ret := nvml.DeviceGetHandleByIndex(i)
			if ret != nvml.SUCCESS {
				continue // Skip to next device on error
			}

			gpuAttr := metric.WithAttributes(attribute.Int("gpu.index", i))

			if util, ret := dev.GetUtilizationRates(); ret == nvml.SUCCESS {
				o.ObserveFloat64(gpuGauge, float64(util.Gpu), gpuAttr)
			}
			if temp, ret := dev.GetTemperature(nvml.TEMPERATURE_GPU); ret == nvml.SUCCESS {
				o.ObserveFloat64(gpuTempGauge, float64(temp), gpuAttr)
			}
			if memInfo, ret := dev.GetMemoryInfo(); ret == nvml.SUCCESS {
				if memInfo.Total > 0 {
					vramPercent := (float64(memInfo.Used) / float64(memInfo.Total)) * 100.0
					o.ObserveFloat64(gpuVRAMGauge, vramPercent, gpuAttr)
				}
			}
		}
	}
}