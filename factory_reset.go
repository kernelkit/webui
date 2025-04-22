package main

import (
	"log"
	"net/http"
	"time"
)

// factoryResetHandler handles the factory reset page
func factoryResetHandler(w http.ResponseWriter, r *http.Request) {
	renderPage(w, r, "factory-reset", nil)
}

// factoryResetExecuteHandler handles the actual factory reset operation
func factoryResetExecuteHandler(w http.ResponseWriter, r *http.Request) {
	// Log the factory reset request
	username := getUsername(r)
	log.Printf("Factory reset requested by user: %s", username)

	// Set headers to prevent caching
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	// Send a simple acknowledgment response
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Reset command received"))

	// In a real implementation, this would call the sysrepo-to-C-bridge to issue an RPC
	// For now, just simulate a reset process with logging
	go func() {
		log.Printf("Starting simulated factory reset process for user: %s", username)

		// Simulate the reset process taking some time
		time.Sleep(5 * time.Second)
		log.Printf("Factory reset process for user %s completed", username)

		// Simulate the reboot
		log.Printf("Simulating reboot after factory reset")

		// In a real environment, you would trigger the sysrepo RPC here:
		// resetErr := sysrepo.ExecuteRPC("factory-reset")
	}()
}
