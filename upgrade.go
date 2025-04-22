package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// UpgradeInfo holds data for the upgrade page
type UpgradeInfo struct {
	FirmwareVersion string
	BuildDate       string
	ActiveBootslot  string
}

// UpgradeStatus tracks the status of an upgrade
type UpgradeStatus struct {
	Status     string  `json:"status"`
	Progress   float64 `json:"progress"`
	Message    string  `json:"message"`
	ShowReboot bool    `json:"show_reboot"`
	Error      string  `json:"error,omitempty"`
}

var (
	// Global upgrade status that can be queried
	currentUpgrade      UpgradeStatus
	currentUpgradeMutex sync.Mutex
	// Path for storing uploaded files
	uploadDir = "/tmp/upgrade"
)

// upgradeHandler handles the upgrade page
func upgradeHandler(w http.ResponseWriter, r *http.Request) {
	info, err := getUpgradeInfo()
	if err != nil {
		log.Printf("Error getting upgrade info: %v", err)
		http.Error(w, "Failed to get upgrade information", http.StatusInternalServerError)
		return
	}

	renderPage(w, r, "upgrade", info)
}

// getUpgradeInfo retrieves the current firmware information
func getUpgradeInfo() (*UpgradeInfo, error) {
	// In a real system, this would come from actual system information
	// For now, we'll use dummy data or try to get from RAUC

	// Try to get information from RAUC
	fwVersion := "Unknown"
	buildDate := "Unknown"
	activeSlot := "Unknown"

	// Run rauc status to get current system info
	cmd := exec.Command("rauc", "status")
	output, err := cmd.CombinedOutput()
	if err == nil {
		// Parse RAUC output
		outputStr := string(output)

		// Extract firmware version
		if versionLine := extractLine(outputStr, "version="); versionLine != "" {
			fwVersion = strings.TrimPrefix(versionLine, "version=")
		}

		// Extract build date
		if dateLine := extractLine(outputStr, "build="); dateLine != "" {
			buildDate = strings.TrimPrefix(dateLine, "build=")
		}

		// Extract active bootslot
		if bootLine := extractLine(outputStr, "booted from:"); bootLine != "" {
			activeSlot = strings.TrimPrefix(bootLine, "booted from: ")
		}
	} else {
		log.Printf("Error getting RAUC status: %v, using fallback values", err)
		// Fallback values if RAUC is not available
		fwVersion = "1.0.0"
		buildDate = time.Now().Format("2006-01-02 15:04:05")
		activeSlot = "slot A"
	}

	return &UpgradeInfo{
		FirmwareVersion: fwVersion,
		BuildDate:       buildDate,
		ActiveBootslot:  activeSlot,
	}, nil
}

// extractLine extracts a line from multiline text that contains a specified prefix
func extractLine(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if strings.Contains(line, prefix) {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

// downloadConfigHandler handles downloading the current configuration
func downloadConfigHandler(w http.ResponseWriter, r *http.Request) {
	// In a real system, you would generate the actual config file
	// For now, just create a dummy file

	configPath := "/tmp/startup-config.cfg"

	// Create a dummy config file
	content := fmt.Sprintf("# Configuration Backup\n# Generated: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))
	content += "hostname=example-device\n"
	content += "version=1.0.0\n"
	content += "# This is a sample configuration file\n"

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		log.Printf("Error creating config file: %v", err)
		http.Error(w, "Failed to generate configuration file", http.StatusInternalServerError)
		return
	}

	// Set headers for file download
	w.Header().Set("Content-Disposition", "attachment; filename=startup-config.cfg")
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))

	// Serve the file
	http.ServeFile(w, r, configPath)
}

// uploadFirmwareHandler handles firmware upload and installation
func uploadFirmwareHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Upload firmware request received")

	// Create upload directory if it doesn't exist
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		log.Printf("Error creating upload directory: %v", err)
		http.Error(w, "Failed to prepare for upload", http.StatusInternalServerError)
		return
	}

	// Parse multipart form (max 500MB)
	if err := r.ParseMultipartForm(500 << 20); err != nil {
		log.Printf("Error parsing form: %v", err)
		http.Error(w, "Failed to parse upload form", http.StatusBadRequest)
		return
	}

	// Get firmware file
	firmwareFile, firmwareHeader, err := r.FormFile("firmware")
	if err != nil {
		log.Printf("Error getting firmware file: %v", err)
		http.Error(w, "No firmware file provided", http.StatusBadRequest)
		return
	}
	defer firmwareFile.Close()

	log.Printf("Firmware file received: %s", firmwareHeader.Filename)

	// Validate firmware file
	if !strings.HasSuffix(strings.ToLower(firmwareHeader.Filename), ".pkg") {
		log.Printf("Invalid firmware file extension: %s", firmwareHeader.Filename)
		http.Error(w, "Invalid firmware file. File must have .pkg extension", http.StatusBadRequest)
		return
	}

	// Save firmware file
	firmwarePath := filepath.Join(uploadDir, "firmware.pkg")
	firmwareOut, err := os.Create(firmwarePath)
	if err != nil {
		log.Printf("Error creating firmware file: %v", err)
		http.Error(w, "Failed to save firmware file", http.StatusInternalServerError)
		return
	}
	defer firmwareOut.Close()

	if _, err := io.Copy(firmwareOut, firmwareFile); err != nil {
		log.Printf("Error saving firmware file: %v", err)
		http.Error(w, "Failed to save firmware file", http.StatusInternalServerError)
		return
	}

	log.Printf("Firmware file saved to %s", firmwarePath)

	// Check if config file was also uploaded
	configPath := ""
	configFile, configHeader, err := r.FormFile("config")
	if err == nil {
		defer configFile.Close()

		// Validate config file
		if !strings.HasSuffix(strings.ToLower(configHeader.Filename), ".cfg") {
			log.Printf("Invalid config file extension: %s", configHeader.Filename)
			http.Error(w, "Invalid configuration file. File must have .cfg extension", http.StatusBadRequest)
			return
		}

		// Save config file
		configPath = filepath.Join(uploadDir, "config.cfg")
		configOut, err := os.Create(configPath)
		if err != nil {
			log.Printf("Error creating config file: %v", err)
			http.Error(w, "Failed to save configuration file", http.StatusInternalServerError)
			return
		}
		defer configOut.Close()

		if _, err := io.Copy(configOut, configFile); err != nil {
			log.Printf("Error saving config file: %v", err)
			http.Error(w, "Failed to save configuration file", http.StatusInternalServerError)
			return
		}

		log.Printf("Config file saved to %s", configPath)
	}

	// Reset upgrade status
	currentUpgradeMutex.Lock()
	currentUpgrade = UpgradeStatus{
		Status:   "uploading",
		Progress: 0,
		Message:  "Files uploaded successfully, starting installation...",
	}
	currentUpgradeMutex.Unlock()

	// Start the upgrade process in a goroutine
	go startUpgradeProcess(firmwarePath, configPath)

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(currentUpgrade)
	log.Printf("Upload firmware response sent, starting upgrade process")
}

// upgradeStatusHandler returns the current upgrade status
func upgradeStatusHandler(w http.ResponseWriter, r *http.Request) {
	currentUpgradeMutex.Lock()
	status := currentUpgrade
	currentUpgradeMutex.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// startUpgradeProcess simulates the RAUC upgrade process
func startUpgradeProcess(firmwarePath, configPath string) {
	log.Printf("Starting upgrade process for %s", firmwarePath)

	// Update status to installing
	updateUpgradeStatus("installing", 5, "Starting installation process...")

	// Validate firmware package (in a real system, this would check signatures, etc.)
	log.Printf("Validating firmware package: %s", firmwarePath)
	time.Sleep(2 * time.Second)
	updateUpgradeStatus("installing", 10, "Validating firmware package...")

	// Check if RAUC is available
	_, err := exec.LookPath("rauc")
	usingRauc := err == nil

	if usingRauc {
		// Using actual RAUC command
		log.Printf("RAUC available, performing actual installation")
		updateUpgradeStatus("installing", 15, "Starting RAUC installation...")

		// Set up a command to run RAUC
		cmd := exec.Command("rauc", "install", firmwarePath)

		// Start the command
		err := cmd.Start()
		if err != nil {
			log.Printf("Error starting RAUC: %v", err)
			updateUpgradeStatus("error", 15, "Failed to start installation")
			return
		}

		// Track progress (in a real system, you would parse RAUC's output for progress)
		// For this example, we'll simulate progress
		for i := 20; i <= 90; i += 5 {
			updateUpgradeStatus("installing", float64(i), fmt.Sprintf("Installing firmware (%d%%)", i))

			// Check if process is still running
			if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
				break
			}

			time.Sleep(1 * time.Second)
		}

		// Wait for command to complete
		err = cmd.Wait()
		if err != nil {
			log.Printf("RAUC installation failed: %v", err)
			updateUpgradeStatus("error", 90, fmt.Sprintf("Installation failed: %v", err))
			return
		}

	} else {
		// Simulate RAUC installation process
		log.Printf("RAUC not available, simulating installation process")
		updateUpgradeStatus("installing", 15, "Preparing installation environment...")

		// Simulate installation progress
		for i := 20; i <= 90; i += 5 {
			updateUpgradeStatus("installing", float64(i), fmt.Sprintf("Installing firmware (%d%%)", i))
			log.Printf("Installation progress: %d%%", i)
			time.Sleep(1 * time.Second)
		}
	}

	// If config file was provided, apply it
	if configPath != "" {
		updateUpgradeStatus("configuring", 95, "Applying configuration...")
		log.Printf("Applying configuration: %s", configPath)
		time.Sleep(2 * time.Second)
	}

	// Complete the upgrade
	updateUpgradeStatus("completed", 100, "Installation completed successfully")
	log.Printf("Upgrade process completed")
}

// updateUpgradeStatus updates the current upgrade status
func updateUpgradeStatus(status string, progress float64, message string) {
	currentUpgradeMutex.Lock()
	defer currentUpgradeMutex.Unlock()

	currentUpgrade.Status = status
	currentUpgrade.Progress = progress
	currentUpgrade.Message = message

	// Set ShowReboot to true when installation is complete
	if status == "completed" {
		currentUpgrade.ShowReboot = true
	}

	// Set Error field for error status
	if status == "error" && !strings.HasPrefix(message, "Error:") {
		currentUpgrade.Error = "Error: " + message
	}

	log.Printf("Upgrade status updated: %s, %.0f%%, %s", status, progress, message)
}

// rebootHandler initiates a system reboot
func rebootHandler(w http.ResponseWriter, r *http.Request) {
	// Log the reboot request
	log.Println("Reboot requested by user:", getUsername(r))

	// Set appropriate response headers
	w.Header().Set("Content-Type", "application/json")

	// Return a success response
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "rebooting",
		"message": "System is rebooting...",
	})

	// In a real system, you would trigger the reboot here
	go func() {
		log.Println("Simulating system reboot...")
		// In a real environment, you would execute a reboot command here
		time.Sleep(2 * time.Second)
		log.Println("Reboot simulation complete")
	}()
}
