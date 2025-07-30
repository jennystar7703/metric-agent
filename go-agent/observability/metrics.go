// In go-agent/observability/metrics.go

package observability

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"

	// REMOVED: "github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

// This function remains unchanged.
func initMeterProvider(ctx context.Context, res *resource.Resource) (*sdkmetric.MeterProvider, error) {
	exporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithInsecure(),
		otlpmetricgrpc.WithEndpoint(otelExporterEndpoint),
	)
	if err != nil {
		return nil, err
	}
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(otelMetricInterval))),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(meterProvider)
	return meterProvider, nil
}

func registerOtelMetrics(mp *sdkmetric.MeterProvider) error {
	meter := mp.Meter(otelAgentName)

	// Define gauges for ALL your telemetry data
	cpuGauge, _ := meter.Float64ObservableGauge("system.cpu.utilization", metric.WithDescription("CPU Utilization Percentage"))
	memGauge, _ := meter.Float64ObservableGauge("system.memory.utilization", metric.WithDescription("Memory Utilization Percentage"))
	gpuGauge, _ := meter.Float64ObservableGauge("system.gpu.utilization", metric.WithDescription("GPU Utilization Percentage"))
	gpuTempGauge, _ := meter.Float64ObservableGauge("system.gpu.temperature", metric.WithDescription("GPU Temperature in Celsius"))
	gpuVRAMGauge, _ := meter.Float64ObservableGauge("system.gpu.vram.utilization", metric.WithDescription("GPU VRAM Utilization Percentage"))
	storageUsedGauge, _ := meter.Int64ObservableGauge("system.storage.used_gb", metric.WithDescription("Used Storage on root partition in GB"))
	ssdHealthGauge, _ := meter.Float64ObservableGauge("system.ssd.health_percent", metric.WithDescription("SSD Health Percentage (100 - Percentage Used)"))
	harddiskUsedPercentGauge, _ := meter.Float64ObservableGauge("system.harddisk.used_percent", metric.WithDescription("Total used hard disk space percentage across all mounts"))

	_, err := meter.RegisterCallback(func(ctx context.Context, o metric.Observer) error {
		// CPU Usage
		if percentages, err := cpu.Percent(0, false); err == nil && len(percentages) > 0 {
			o.ObserveFloat64(cpuGauge, percentages[0])
		}

		// Memory Usage
		if vmStat, err := mem.VirtualMemory(); err == nil {
			o.ObserveFloat64(memGauge, vmStat.UsedPercent)
		}

		// Storage Used (on root partition)
		if usage, err := disk.Usage("/"); err == nil {
			o.ObserveInt64(storageUsedGauge, int64(usage.Used/(1024*1024*1024)))
		}

		// Calculate and observe total hard disk usage percentage
		var totalDiskSpace uint64 = 0
		var totalUsedSpace uint64 = 0
		if partitions, err := disk.Partitions(true); err == nil {
			for _, p := range partitions {
				// A simple whitelist to avoid temporary/system filesystems
				if strings.HasPrefix(p.Fstype, "ext") || strings.HasPrefix(p.Fstype, "xfs") || strings.HasPrefix(p.Fstype, "btrfs") || strings.HasPrefix(p.Fstype, "nfs") {
					if usage, err := disk.Usage(p.Mountpoint); err == nil {
						totalDiskSpace += usage.Total
						totalUsedSpace += usage.Used
					}
				}
			}
		}
		if totalDiskSpace > 0 {
			usedPercent := (float64(totalUsedSpace) / float64(totalDiskSpace)) * 100.0
			o.ObserveFloat64(harddiskUsedPercentGauge, usedPercent)
		} else {
			o.ObserveFloat64(harddiskUsedPercentGauge, 0.0)
		}

		// --- THIS IS THE KEY CHANGE ---
		// This single function call will either run the real GPU code or the empty
		// stub, depending on which file was compiled.
		observeGpuMetrics(o, gpuGauge, gpuTempGauge, gpuVRAMGauge)
		// --- END KEY CHANGE ---

		// SSD Health (per drive)
		if devices, err := discoverBlockDevices(); err == nil {
			for _, devicePath := range devices {
				if !strings.HasPrefix(devicePath, "/dev/nvme") {
					continue
				}
				deviceAttr := metric.WithAttributes(attribute.String("device.path", devicePath))
				healthPercentage := 0.0
				cmd := exec.Command("smartctl", "-A", "-j", devicePath)
				output, err := cmd.Output()
				if err == nil {
					var smartctlData struct {
						NvmeSmartHealthLog struct {
							PercentageUsed int `json:"percentage_used"`
						} `json:"nvme_smart_health_information_log"`
					}
					if json.Unmarshal(output, &smartctlData) == nil {
						healthPercentage = 100.0 - float64(smartctlData.NvmeSmartHealthLog.PercentageUsed)
					}
				}
				o.ObserveFloat64(ssdHealthGauge, healthPercentage, deviceAttr)
			}
		}
		return nil
	},
		cpuGauge,
		memGauge,
		gpuGauge,
		gpuTempGauge,
		gpuVRAMGauge,
		storageUsedGauge,
		ssdHealthGauge,
		harddiskUsedPercentGauge,
	)
	return err
}

// This function remains unchanged.
func discoverBlockDevices() ([]string, error) {
	files, err := os.ReadDir("/sys/block")
	if err != nil {
		return nil, err
	}
	var devices []string
	for _, file := range files {
		name := file.Name()
		if strings.HasPrefix(name, "sd") || strings.HasPrefix(name, "nvme") {
			devices = append(devices, "/dev/"+name)
		}
	}
	return devices, nil
}