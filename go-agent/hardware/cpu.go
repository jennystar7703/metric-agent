//go:build !gpu

package hardware

// This is the FAKE implementation for CPU-only machines
func InitializeHardware() error {
	// Do nothing for CPU builds
	return nil
}

func ShutdownHardware() {
	// Do nothing for CPU builds
}

func getGpuSpecs(specs *HardwareSpecs) {
	// For CPU builds, we report 0 GPU specs
	specs.GPUCount = "0"
	specs.GPUModel = "N/A"
	specs.GPUVRAMGB = "0"
}
