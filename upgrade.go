package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	Status      string  `json:"status"`
	Progress    float64 `json:"progress"`
	Message     string  `json:"message"`
	ShowReboot  bool    `json:"show_reboot"`
	Error       string  `json:"error,omitempty"`
	TimeElapsed string  `json:"time_elapsed,omitempty"`
}

var (
	// Global upgrade status that can be queried
	currentUpgrade UpgradeStatus
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
	// For now, we'll use dummy data

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
	// For this example, we'll just create a dummy file

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
	// Create upload directory if it doesn't exist
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		log.Printf("Error creating upload directory: %v", err)
		renderUpgradeError(w, "Failed to prepare for upload")
		return
	}

	// Parse multipart form (max 500MB)
	if err := r.ParseMultipartForm(500 << 20); err != nil {
		log.Printf("Error parsing form: %v", err)
		renderUpgradeError(w, "Failed to parse upload form")
		return
	}

	// Get firmware file
	firmwareFile, firmwareHeader, err := r.FormFile("firmware")
	if err != nil {
		log.Printf("Error getting firmware file: %v", err)
		renderUpgradeError(w, "No firmware file provided")
		return
	}
	defer firmwareFile.Close()

	// Validate firmware file
	if !strings.HasSuffix(strings.ToLower(firmwareHeader.Filename), ".pkg") {
		renderUpgradeError(w, "Invalid firmware file. File must have .pkg extension")
		return
	}

	// Save firmware file
	firmwarePath := filepath.Join(uploadDir, "firmware.pkg")
	firmwareOut, err := os.Create(firmwarePath)
	if err != nil {
		log.Printf("Error creating firmware file: %v", err)
		renderUpgradeError(w, "Failed to save firmware file")
		return
	}
	defer firmwareOut.Close()

	if _, err := io.Copy(firmwareOut, firmwareFile); err != nil {
		log.Printf("Error saving firmware file: %v", err)
		renderUpgradeError(w, "Failed to save firmware file")
		return
	}

	// Check if config file was also uploaded
	configPath := ""
	configFile, configHeader, err := r.FormFile("config")
	if err == nil {
		defer configFile.Close()

		// Validate config file
		if !strings.HasSuffix(strings.ToLower(configHeader.Filename), ".cfg") {
			renderUpgradeError(w, "Invalid configuration file. File must have .cfg extension")
			return
		}

		// Save config file
		configPath = filepath.Join(uploadDir, "config.cfg")
		configOut, err := os.Create(configPath)
		if err != nil {
			log.Printf("Error creating config file: %v", err)
			renderUpgradeError(w, "Failed to save configuration file")
			return
		}
		defer configOut.Close()

		if _, err := io.Copy(configOut, configFile); err != nil {
			log.Printf("Error saving config file: %v", err)
			renderUpgradeError(w, "Failed to save configuration file")
			return
		}

		log.Printf("Config file saved to %s", configPath)
	}

	// Reset upgrade status
	currentUpgrade = UpgradeStatus{
		Status:   "uploading",
		Progress: 0,
		Message:  "Files uploaded successfully, starting installation...",
	}

	// Start the upgrade process in a goroutine
	go startUpgradeProcess(firmwarePath, configPath)

	// Return initial response
	renderUpgradeStatus(w)
}

// renderUpgradeError renders an error message
func renderUpgradeError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "text/html")

	html := `<div class="alert alert-danger" role="alert">
		<i class="bi bi-exclamation-circle-fill me-2"></i>
		<strong>Error:</strong> %s
	</div>`

	fmt.Fprintf(w, html, message)
}

// renderUpgradeStatus renders the current upgrade status
func renderUpgradeStatus(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html")

	var html string

	if currentUpgrade.Status == "error" {
		html = `<div class="alert alert-danger" role="alert">
			<i class="bi bi-exclamation-circle-fill me-2"></i>
			<strong>Error:</strong> %s
		</div>`
		fmt.Fprintf(w, html, currentUpgrade.Error)
		return
	}

	html = `<div class="card">
		<div class="card-header">
			<h5>Installation Progress</h5>
		</div>
		<div class="card-body">
			<div class="mb-3">
				<strong>Status:</strong> %s
			</div>
			<div class="progress mb-3" style="height: 25px;">
				<div class="progress-bar %s" role="progressbar" style="width: %.0f%%;" 
					 aria-valuenow="%.0f" aria-valuemin="0" aria-valuemax="100">%.0f%%</div>
			</div>
			<div class="mb-3">
				<strong>Message:</strong> %s
			</div>
			%s
			<div class="mb-3">
				<small class="text-muted">Time elapsed: %s</small>
			</div>
			<div class="mt-3" id="refresh-container">
				<!-- Auto-refresh for progress updates -->
				<div hx-get="/upgrade-status" 
					 hx-trigger="every 2s" 
					 hx-target="#upgrade-status" 
					 hx-swap="innerHTML">
				</div>
			</div>
		</div>
	</div>`

	// Additional class for progress bar
	progressClass := ""
	if currentUpgrade.Progress < 100 {
		progressClass = "progress-bar-striped progress-bar-animated"
	}

	// Reboot button if upgrade is complete
	rebootBtn := ""
	if currentUpgrade.ShowReboot {
		rebootBtn = `<div class="alert alert-success mb-3">
			<i class="bi bi-check-circle-fill me-2"></i>
			Installation complete! Please reboot to apply changes.
			<div class="mt-2">
				<button class="btn btn-success" hx-post="/reboot" hx-target="#upgrade-status">
					<i class="bi bi-arrow-clockwise me-2"></i>Reboot System
				</button>
			</div>
		</div>`
	}

	fmt.Fprintf(w, html,
		currentUpgrade.Status,
		progressClass,
		currentUpgrade.Progress,
		currentUpgrade.Progress,
		currentUpgrade.Progress,
		currentUpgrade.Message,
		rebootBtn,
		currentUpgrade.TimeElapsed)
}

// upgradeStatusHandler returns the current upgrade status
func upgradeStatusHandler(w http.ResponseWriter, r *http.Request) {
	renderUpgradeStatus(w)
}

// startUpgradeProcess simulates the RAUC upgrade process
func startUpgradeProcess(firmwarePath, configPath string) {
	startTime := time.Now()

	// Update status to installing
	currentUpgrade.Status = "installing"
	currentUpgrade.Message = "Starting installation process..."

	// Validate firmware package (in a real system, this would check signatures, etc.)
	log.Printf("Validating firmware package: %s", firmwarePath)
	time.Sleep(2 * time.Second)

	// Check if RAUC is available
	_, err := exec.LookPath("rauc")
	usingRauc := err == nil

	if usingRauc {
		// Using actual RAUC command
		log.Printf("RAUC available, performing actual installation")

		// Set up a command to run RAUC
		cmd := exec.Command("rauc", "install", firmwarePath)

		// Start the command
		err := cmd.Start()
		if err != nil {
			log.Printf("Error starting RAUC: %v", err)
			currentUpgrade.Status = "error"
			currentUpgrade.Error = fmt.Sprintf("Failed to start RAUC: %v", err)
			return
		}

		// Track progress (in a real system, you would parse RAUC's output for progress)
		// For this example, we'll simulate progress
		for i := 0; i <= 100; i += 5 {
			currentUpgrade.Progress = float64(i)
			currentUpgrade.Message = fmt.Sprintf("Installing firmware (%.0f%%)", currentUpgrade.Progress)
			currentUpgrade.TimeElapsed = time.Since(startTime).Round(time.Second).String()

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
			currentUpgrade.Status = "error"
			currentUpgrade.Error = fmt.Sprintf("Installation failed: %v", err)
			return
		}

	} else {
		// Simulate RAUC installation process
		log.Printf("RAUC not available, simulating installation process")

		// Simulate installation progress
		for i := 0; i <= 100; i += 5 {
			currentUpgrade.Progress = float64(i)
			currentUpgrade.Message = fmt.Sprintf("Installing firmware (%.0f%%)", currentUpgrade.Progress)
			currentUpgrade.TimeElapsed = time.Since(startTime).Round(time.Second).String()

			time.Sleep(1 * time.Second)
		}
	}

	// If config file was provided, apply it
	if configPath != "" {
		currentUpgrade.Message = "Applying configuration..."
		log.Printf("Applying configuration: %s", configPath)
		time.Sleep(2 * time.Second)
	}

	// Complete the upgrade
	currentUpgrade.Status = "completed"
	currentUpgrade.Progress = 100
	currentUpgrade.Message = "Installation completed successfully"
	currentUpgrade.ShowReboot = true
	currentUpgrade.TimeElapsed = time.Since(startTime).Round(time.Second).String()

	log.Printf("Upgrade process completed in %s", currentUpgrade.TimeElapsed)
}

// rebootHandler initiates a system reboot
func rebootHandler(w http.ResponseWriter, r *http.Request) {
	// Set a delayed reboot (in a real system)
	log.Println("Reboot requested")

	// Return a response to the user
	w.Header().Set("Content-Type", "text/html")

	html := `<div class="alert alert-info" role="alert">
		<i class="bi bi-arrow-clockwise me-2"></i>
		<strong>Rebooting...</strong> The system is rebooting. This page will refresh in 30 seconds.
	</div>
	<script>
		// Remove the auto-refresh for status updates
		document.getElementById('refresh-container').innerHTML = '';
		
		// Redirect to status page after a delay
		setTimeout(function() {
			window.location.href = '/status';
		}, 30000);
	</script>`

	fmt.Fprint(w, html)

	// In a real system, you would trigger a reboot here
	// For example: go triggerSystemReboot()
}

// triggerSystemReboot would actually reboot the system
func triggerSystemReboot() {
	// Wait a moment to allow the HTTP response to be sent
	time.Sleep(2 * time.Second)

	// Execute the reboot command
	cmd := exec.Command("reboot")
	err := cmd.Run()
	if err != nil {
		log.Printf("Error executing reboot command: %v", err)
	}
}
