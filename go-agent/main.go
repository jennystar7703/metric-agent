package main

import (
	"context"
	"log"

	// --- ADD THIS IMPORT ---
	// Assumes your go.mod file defines the module as 'ndp-agent'
	"ndp-agent/hardware"

	// Local package for OTel setup
	"ndp-agent/observability"
)

func main() {
	log.Println("Starting Unified NDP Agent...")

	// --- UPDATE THE FUNCTION CALLS ---
	if err := hardware.InitializeHardware(); err != nil {
		log.Printf("Warning: Hardware initialization failed: %v. Some metrics may be unavailable.", err)
	} else {
		defer hardware.ShutdownHardware()
	}

	// The verification function now needs to be passed to main
	if err := verifyAndSync(hardware.GetHardwareSpecs); err != nil {
		log.Fatalf("Critical error during application setup: %v", err)
	}

	// This part is now handled inside verifyAndSync
	// if err := loadOrCreateState(); err != nil { ... }

	otelShutdown, err := observability.InitProviders(context.Background(), hardware.GetNodeID())
	if err != nil {
		log.Fatalf("Failed to initialize observability providers: %v", err)
	}
	defer func() {
		if err := otelShutdown(context.Background()); err != nil {
			log.Printf("Error shutting down observability providers: %v", err)
		}
	}()
	log.Println("Observability components initialized successfully.")

	log.Println("Node verified and hardware synced successfully!")
	log.Println("Agent is running. OTel is collecting metrics in the background. Press Ctrl+C to exit.")
	select {}
}

// New function to orchestrate setup
func verifyAndSync(getSpecsFunc func() (hardware.HardwareSpecs, error)) error {
	if err := hardware.LoadOrCreateState(); err != nil {
		return err
	}
	
	if err := hardware.VerifyAndSyncHardware(getSpecsFunc); err != nil {
		return err
	}
	
	return nil
}
