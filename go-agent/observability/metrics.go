package observability

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
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
	// --- NEW ---: Gauge for VRAM percentage
	gpuVRAMGauge, _ := meter.Float64ObservableGauge("system.gpu.vram.utilization", metric.WithDescription("GPU VRAM Utilization Percentage"))
	storageUsedGauge, _ := meter.Int64ObservableGauge("system.storage.used_gb", metric.WithDescription("Used Storage in GB"))
	//ssdHealthGauge, _ := meter.Int64ObservableGauge("system.ssd.health_passed", metric.WithDescription("SSD SMART health check status (1 = passed, 0 = failed)"))
	ssdHealthGauge, _ := meter.Float64ObservableGauge("system.ssd.health_percent", metric.WithDescription("SSD Health Percentage (100 - Percentage Used)"))


	_, err := meter.RegisterCallback(func(ctx context.Context, o metric.Observer) error {
		// CPU Usage
		if percentages, err := cpu.Percent(0, false); err == nil && len(percentages) > 0 {
			o.ObserveFloat64(cpuGauge, percentages[0])
		}

		// Memory Usage
		if vmStat, err := mem.VirtualMemory(); err == nil {
			o.ObserveFloat64(memGauge, vmStat.UsedPercent)
		}
		
		// Storage Used
		if usage, err := disk.Usage("/"); err == nil {
			o.ObserveInt64(storageUsedGauge, int64(usage.Used/(1024*1024*1024)))
		}

		// GPU Metrics (per GPU)
		if count, ret := nvml.DeviceGetCount(); ret == nvml.SUCCESS {
			for i := 0; i < int(count); i++ {
				dev, ret := nvml.DeviceGetHandleByIndex(i)
				if ret != nvml.SUCCESS {
					continue // Skip to next device on error
				}

				gpuAttr := metric.WithAttributes(attribute.Int("gpu.index", i))

				// GPU Core Utilization
				if util, ret := dev.GetUtilizationRates(); ret == nvml.SUCCESS {
					o.ObserveFloat64(gpuGauge, float64(util.Gpu), gpuAttr)
				}

				// GPU Temperature
				if temp, ret := dev.GetTemperature(nvml.TEMPERATURE_GPU); ret == nvml.SUCCESS {
					o.ObserveFloat64(gpuTempGauge, float64(temp), gpuAttr)
				}

				// --- NEW ---: VRAM Utilization Percentage
				if memInfo, ret := dev.GetMemoryInfo(); ret == nvml.SUCCESS {
					if memInfo.Total > 0 { // Avoid division by zero
						vramPercent := (float64(memInfo.Used) / float64(memInfo.Total)) * 100.0
						o.ObserveFloat64(gpuVRAMGauge, vramPercent, gpuAttr)
					}
				}
			}
		}
		
		/*
		// SSD Health (per drive)
		if devices, err := discoverBlockDevices(); err == nil {
			for _, devicePath := range devices {
				deviceAttr := metric.WithAttributes(attribute.String("device.path", devicePath))
				passed := 0 // Assume failed unless proven otherwise
				cmd := exec.Command("smartctl", "-H", "-j", devicePath)
				output, err := cmd.Output()

				if err == nil {
					var smartctlData struct {
						SmartStatus struct {
							Passed bool `json:"passed"`
						} `json:"smart_status"`
					}
					if json.Unmarshal(output, &smartctlData) == nil && smartctlData.SmartStatus.Passed {
						passed = 1
					}
				}
				o.ObserveInt64(ssdHealthGauge, int64(passed), deviceAttr)
			}
		} */
		if devices, err := discoverBlockDevices(); err == nil {
			for _, devicePath := range devices {
                // Only check NVMe devices for percentage used attribute
                if !strings.HasPrefix(devicePath, "/dev/nvme") {
                    continue
                }

				deviceAttr := metric.WithAttributes(attribute.String("device.path", devicePath))
				healthPercentage := 0.0 // Default to 0% health on error

                // Use -A to get all attributes
				cmd := exec.Command("smartctl", "-A", "-j", devicePath)
				output, err := cmd.Output()

				if err == nil {
					var smartctlData struct {
						NvmeSmartHealthLog struct {
							PercentageUsed int `json:"percentage_used"`
						} `json:"nvme_smart_health_information_log"`
					}
					if json.Unmarshal(output, &smartctlData) == nil {
                        // Health is defined as 100% minus the percentage used
						healthPercentage = 100.0 - float64(smartctlData.NvmeSmartHealthLog.PercentageUsed)
					}
				}
				o.ObserveFloat64(ssdHealthGauge, healthPercentage, deviceAttr)
			}
		}
		return nil 

	},

		// --- UPDATE ---: Add the new gauge to the callback registration
		cpuGauge,
		memGauge,
		gpuGauge,
		gpuTempGauge,
		gpuVRAMGauge,
		storageUsedGauge,
		ssdHealthGauge,
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