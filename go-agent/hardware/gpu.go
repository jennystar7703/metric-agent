//go:build gpu

package hardware

import (
	"fmt"
	"strconv"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// This is the REAL implementation for GPU machines
func InitializeHardware() error {
	if ret := nvml.Init(); ret != nvml.SUCCESS {
		return fmt.Errorf("NVML init failed: %v", nvml.ErrorString(ret))
	}
	return nil
}

func ShutdownHardware() {
	nvml.Shutdown()
}

func getGpuSpecs(specs *HardwareSpecs) {
	if count, ret := nvml.DeviceGetCount(); ret == nvml.SUCCESS && count > 0 {
		specs.GPUCount = strconv.Itoa(count)
		dev, _ := nvml.DeviceGetHandleByIndex(0)
		name, _ := dev.GetName()
		specs.GPUModel = name
		vram, _ := dev.GetMemoryInfo()
		specs.GPUVRAMGB = strconv.Itoa(int(vram.Total / 1024 / 1024 / 1024))
	}
}
