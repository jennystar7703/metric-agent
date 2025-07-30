package hardware

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
)

const (
	ndpApiBaseURL  = "http://211.176.180.172:8080/api/v1"
	stateFilePath  = "/app/agent_state.json"
	hostMountsFile = "/host/mounts"
)

type HardwareSpecs struct {
	NodeName      string `json:"node_name"`
	NanoDcID      string `json:"nanodc_id"`
	CPUModel      string `json:"cpu_model"`
	CPUCores      string `json:"cpucores"`
	CPUCount      string `json:"cpu_count"`
	GPUModel      string `json:"gpu_model"`
	GPUCount      string `json:"gpu_count"`
	GPUVRAMGB     string `json:"gpu_vram_gb"`
	TotalRAMGB    string `json:"total_ram_gb"`
	StorageType   string `json:"storage_type"`
	StorageTotalGB string `json:"storage_total_gb"`
	TotalHarddiskGB string `json:"total_harddisk_gb"`
	NVMeCount     string `json:"nvme_count"`
}

type VerificationRequest struct {
	NodeID        string        `json:"node_id"`
	HardwareSpecs HardwareSpecs `json:"hardware_specs"`
}

type AgentState struct {
	NodeID   string `json:"node_id"`
	NodeName string `json:"node_name"`
	NanoDcID string `json:"nanodc_id"`
}

var agentState AgentState
var httpClient = &http.Client{Timeout: 10 * time.Second}

func LoadOrCreateState() error {
	data, err := os.ReadFile(stateFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("First time setup: 'agent_state.json' not found. Generating new state...")
			nodeName := os.Getenv("NDP_NODE_NAME")
			if nodeName == "" {
				id := uuid.New()
				nodeName = fmt.Sprintf("node-%s", id.String()[:8])
				log.Printf("Warning: NDP_NODE_NAME not set. Using default: %s", nodeName)
			}
			nanoDcID := os.Getenv("NDP_NANODC_ID")
			if nanoDcID == "" {
				log.Printf("Warning: NDP_NANODC_ID not set. It will be blank.")
			}
			agentState = AgentState{
				NodeID:   uuid.New().String(),
				NodeName: nodeName,
				NanoDcID: nanoDcID,
			}
			newStateData, err := json.MarshalIndent(agentState, "", "  ")
			if err != nil { return fmt.Errorf("failed to marshal new state: %w", err) }
			if err := os.WriteFile(stateFilePath, newStateData, 0644); err != nil {
				return fmt.Errorf("failed to write state file: %w", err)
			}
			log.Printf("New state file created with Node ID: %s, Node Name: %s, NanoDC ID: %s", agentState.NodeID, agentState.NodeName, agentState.NanoDcID)
			return nil
		}
		return fmt.Errorf("failed to read state file: %w", err)
	}
	if err = json.Unmarshal(data, &agentState); err != nil {
		return fmt.Errorf("failed to unmarshal state file: %w", err)
	}
	if agentState.NodeID == "" {
		return fmt.Errorf("state file is invalid: missing node_id")
	}
	log.Printf("Agent state loaded: node_id=%s, node_name=%s, nanodc_id=%s", agentState.NodeID, agentState.NodeName, agentState.NanoDcID)
	return nil
}

func GetTotalDiskUsage() (total uint64, used uint64, err error) {
	fsWhitelist := map[string]bool{"nfs": true, "nfs4": true}
	file, err := os.Open(hostMountsFile)
	if err != nil {
		return 0, 0, fmt.Errorf("could not open host mounts file at %s: %w", hostMountsFile, err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	seenDevices := make(map[string]bool)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 { continue }
		sourceDevice := fields[0]
		mountPoint := fields[1]
		fsType := fields[2]
		if fsWhitelist[fsType] && !seenDevices[sourceDevice] {
			usage, diskErr := disk.Usage(mountPoint)
			if diskErr != nil {
				log.Printf("Warning: Could not get usage for mount point %s: %v", mountPoint, diskErr)
				continue
			}
			total += usage.Total
			used += usage.Used
			seenDevices[sourceDevice] = true
		}
	}
	return total, used, nil
}

func GetHardwareSpecs() (HardwareSpecs, error) {
	var specs HardwareSpecs
	specs.NodeName = agentState.NodeName
	specs.NanoDcID = agentState.NanoDcID
	if cpuInfo, err := cpu.Info(); err == nil && len(cpuInfo) > 0 {
		specs.CPUModel = cpuInfo[0].ModelName
		physicalIDs := make(map[string]bool)
		for _, info := range cpuInfo {
			physicalIDs[info.PhysicalID] = true
		}
		specs.CPUCount = strconv.Itoa(len(physicalIDs))
	}
	if coreCount, err := cpu.Counts(true); err == nil {
		specs.CPUCores = strconv.Itoa(coreCount)
	}
	if vmStat, err := mem.VirtualMemory(); err == nil {
		specs.TotalRAMGB = strconv.Itoa(int(vmStat.Total / 1024 / 1024 / 1024))
	}
	getGpuSpecs(&specs)
	if usage, err := disk.Usage("/"); err == nil {
		specs.StorageTotalGB = strconv.Itoa(int(usage.Total / 1024 / 1024 / 1024))
	} else {
		log.Printf("Warning: Could not get usage for root partition '/': %v", err)
	}
	log.Println("Calculating total hard disk space from host mounts...")
	totalDiskSpace, _, err := GetTotalDiskUsage()
	if err != nil {
		log.Println(err)
	}
	specs.TotalHarddiskGB = strconv.Itoa(int(totalDiskSpace / 1024 / 1024 / 1024))
	log.Printf("Final calculated total_harddisk_gb: %s", specs.TotalHarddiskGB)
	specs.StorageType = "NVMe"
	files, _ := os.ReadDir("/sys/block")
	nvmeCount := 0
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "nvme") {
			nvmeCount++
		}
	}
	specs.NVMeCount = strconv.Itoa(nvmeCount)
	return specs, nil
}

func VerifyAndSyncHardware(getSpecsFunc func() (HardwareSpecs, error)) error {
	log.Println("Verifying node and syncing hardware specs...")
	specs, err := getSpecsFunc()
	if err != nil {
		return fmt.Errorf("could not collect hardware specs: %w", err)
	}
	reqBody := VerificationRequest{NodeID: agentState.NodeID, HardwareSpecs: specs}
	jsonData, _ := json.MarshalIndent(reqBody, "", "  ")
	log.Printf("--- Sending Verification/Sync JSON ---\n%s\n", string(jsonData))
	url := fmt.Sprintf("%s/nodes/register", ndpApiBaseURL)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
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

func GetNodeID() string {
	return agentState.NodeID
}