package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
	"strings"
	// "os/exec"

	// Local package for OTel setup
	"ndp-agent/observability"

	// NVIDIA and system libraries
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
)

// --- CONFIGURATION ---
const (
	ndpApiBaseURL = "http://211.176.180.172:8080/api/v1"
	stateFilePath = "/app/agent_state.json"
)

// --- DATA STRUCTURES (Only for hardware sync) ---
type HardwareSpecs struct {
	CPUModel       string `json:"cpu_model"`
	CPUCores       string `json:"cpucores"`
	CPUCount       string `json:"cpu_count"`       // --- NEW ---: Number of physical CPU sockets
	GPUModel       string `json:"gpu_model"`
	GPUCount       string `json:"gpu_count"`       // --- NEW ---
	GPUVRAMGB      string `json:"gpu_vram_gb"`
	TotalRAMGB     string `json:"total_ram_gb"`
	StorageType    string `json:"storage_type"`
	StorageTotalGB string `json:"storage_total_gb"`
	NVMeCount      string `json:"nvme_count"` // --- CHANGED ---: Replaced NVMeDevices list with a simple count
}


type VerificationRequest struct {
	NodeID        string        `json:"node_id"`
	HardwareSpecs HardwareSpecs `json:"hardware_specs"`
}

type AgentState struct {
	NodeID string `json:"node_id"`
}


var agentState AgentState
var httpClient = &http.Client{Timeout: 10 * time.Second}

// --- MAIN ORCHESTRATOR ---
func main() {
	log.Println("Starting Unified NDP Agent...")

	if ret := nvml.Init(); ret != nvml.SUCCESS {
		log.Printf("Warning: NVML init failed: %v. GPU metrics will be unavailable.", nvml.ErrorString(ret))
	} else {
		defer nvml.Shutdown()
	}

	found, err := loadState()
	if err != nil {
		log.Fatalf("Critical error loading agent state: %v", err)
	}
	if !found {
		log.Fatalf("CRITICAL: '%s' not found. This agent must be pre-provisioned with a Node ID. Please create the file and restart.", stateFilePath)
	}

    // Initialize OTel providers AFTER loading state to get nodeID
	otelShutdown, err := observability.InitProviders(context.Background(), agentState.NodeID)
	if err != nil {
		log.Fatalf("Failed to initialize observability providers: %v", err)
	}
	defer func() {
		if err := otelShutdown(context.Background()); err != nil {
			log.Printf("Error shutting down observability providers: %v", err)
		}
	}()
	log.Println("Observability components initialized successfully.")

    // The one-time hardware sync remains the same
	if err := verifyAndSyncHardware(); err != nil {
		log.Fatalf("Failed to verify with backend. Please check network and backend status. Error: %v", err)
	}
	log.Println("Node verified and hardware synced successfully!")

	log.Println("Agent is running. OTel is collecting metrics in the background. Press Ctrl+C to exit.")
	select {} // Block forever, OTel periodic reader does the work.
}

// --- HELPER FUNCTIONS (Only for hardware sync) ---

func loadState() (bool, error) {
	// ... this function remains the same
	data, err := os.ReadFile(stateFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if err = json.Unmarshal(data, &agentState); err != nil {
		return false, err
	}
	if agentState.NodeID == "" {
		return false, fmt.Errorf("state file is invalid: missing node_id")
	}
	log.Printf("Agent state loaded successfully for node_id: %s", agentState.NodeID)
	return true, nil
}

func discoverNVMeDevices() ([]string, error) {
	files, err := os.ReadDir("/sys/block")
	if err != nil {
		return nil, err
	}
	var devices []string
	for _, file := range files {
		name := file.Name()
		// We filter specifically for devices named like 'nvmeXn1'
		if strings.HasPrefix(name, "nvme") {
			devices = append(devices, "/dev/"+name)
		}
	}
	return devices, nil
}

func getHardwareSpecs() (HardwareSpecs, error) {
	var specs HardwareSpecs

	// --- NEW: Get Physical CPU Count ---
	// gopsutil's cpu.Info() returns a list of all logical cores. Cores on the
	// same physical CPU package share the same "physicalId". We can count the
	// number of unique physical IDs to find the number of CPU sockets.
	if cpuInfo, err := cpu.Info(); err == nil && len(cpuInfo) > 0 {
		specs.CPUModel = cpuInfo[0].ModelName // Model name is the same for all cores

		physicalIDs := make(map[string]bool)
		for _, info := range cpuInfo {
			physicalIDs[info.PhysicalID] = true
		}
		specs.CPUCount = strconv.Itoa(len(physicalIDs))
	}

	// Get total logical core count
	if coreCount, err := cpu.Counts(true); err == nil { // true for logical cores
		specs.CPUCores = strconv.Itoa(coreCount)
	}

	// Get Memory
	if vmStat, err := mem.VirtualMemory(); err == nil {
		specs.TotalRAMGB = strconv.Itoa(int(vmStat.Total / 1024 / 1024 / 1024))
	}

	// Get GPU Info
	if count, ret := nvml.DeviceGetCount(); ret == nvml.SUCCESS && count > 0 {
		specs.GPUCount = strconv.Itoa(count)
		dev, _ := nvml.DeviceGetHandleByIndex(0)
		name, _ := dev.GetName()
		specs.GPUModel = name
		vram, _ := dev.GetMemoryInfo()
		specs.GPUVRAMGB = strconv.Itoa(int(vram.Total / 1024 / 1024 / 1024))
	}

	// Get Storage Info
	if usage, err := disk.Usage("/"); err == nil {
		specs.StorageTotalGB = strconv.Itoa(int(usage.Total / 1024 / 1024 / 1024))
	}
	specs.StorageType = "NVMe"

	// --- NEW: Get NVMe count instead of details ---
	if nvmeDevices, err := discoverNVMeDevices(); err == nil {
		specs.NVMeCount = strconv.Itoa(len(nvmeDevices))
	} else {
		log.Printf("Could not discover NVMe devices: %v", err)
		specs.NVMeCount = "0" // Default to 0 on error
	}

	return specs, nil
}


func verifyAndSyncHardware() error {
	// ... this function remains the same
	log.Println("Verifying node and syncing hardware specs...")
	specs, err := getHardwareSpecs()
	if err != nil {
		return fmt.Errorf("could not collect hardware specs: %w", err)
	}
	reqBody := VerificationRequest{
		NodeID:        agentState.NodeID,
		HardwareSpecs: specs,
	}
	jsonData, err := json.MarshalIndent(reqBody, "", "  ")
	if err != nil {
		return err
	}
	log.Printf("--- Sending Verification/Sync JSON ---\n%s\n", string(jsonData))
	url := fmt.Sprintf("%s/nodes/register", ndpApiBaseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("verification failed with status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
