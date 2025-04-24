package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
)

// SystemInfo holds system information
type SystemInfo struct {
	Hostname       string
	Model          string
	CPUChipset     string
	CPUFrequency   string
	Memory         MemoryData
	Disk           DiskData
	CPUUsage       float64
	LoadAverage    []string
	CPUTemperature string
	CurrentTime    string
	Uptime         string
	VersionInfo    map[string]string
}

// MemoryData holds memory usage information
type MemoryData struct {
	Formatted string
	Percent   float64
}

// DiskData holds disk usage information
type DiskData struct {
	Formatted string
	Percent   float64
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	info, err := getSystemInfo()
	if err != nil {
		log.Printf("Error getting system info: %v", err)
		http.Error(w, "Failed to get system information", http.StatusInternalServerError)
		return
	}

	renderPage(w, r, "status", info)
}

// getSystemInfo gathers system information
func getSystemInfo() (*SystemInfo, error) {
	// Get hostname
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "Unknown"
	}

	// Get model
	model := getSystemModel()

	// Get CPU info
	cpuChipset, cpuFrequency := getCPUInfo()

	// Get memory info
	memoryData := getMemoryInfo()

	// Get disk info
	diskData := getDiskInfo()

	// Get CPU usage
	cpuUsage, _ := getCPUUsage()

	// Get load average
	loadAvg := getLoadAverage()

	// Get CPU temperature
	cpuTemp := getCPUTemperature()

	// Get current time
	currentTime := time.Now().Format(time.RFC1123)

	// Get uptime
	uptime := getUptime()

	// Get version info
	versionInfo := getVersionInfo()

	return &SystemInfo{
		Hostname:       hostname,
		Model:          model,
		CPUChipset:     cpuChipset,
		CPUFrequency:   cpuFrequency,
		Memory:         memoryData,
		Disk:           diskData,
		CPUUsage:       cpuUsage,
		LoadAverage:    loadAvg,
		CPUTemperature: cpuTemp,
		CurrentTime:    currentTime,
		Uptime:         uptime,
		VersionInfo:    versionInfo,
	}, nil
}

// getVersionInfo reads version information from /etc/os-release
func getVersionInfo() map[string]string {
	versionInfo := make(map[string]string)
	relinfo := "/etc/os-release"

	file, err := os.Open(relinfo)
	if err != nil {
		log.Printf("Failed opening %s: %s", relinfo, err.Error())
		versionInfo["ERROR"] = err.Error()
		return versionInfo
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	re := regexp.MustCompile(`^(\w+)=["']?(.*?)["']?$`)

	for scanner.Scan() {
		line := scanner.Text()
		matches := re.FindStringSubmatch(line)
		if len(matches) == 3 {
			versionInfo[matches[1]] = matches[2]
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Failed reading %s: %s", relinfo, err.Error())
		versionInfo["ERROR"] = err.Error()
	}

	return versionInfo
}

// getSystemModel gets the system model from DMI
func getSystemModel() string {
	data, err := ioutil.ReadFile("/sys/devices/virtual/dmi/id/product_family")
	if err != nil {
		return "Unknown"
	}
	return strings.TrimSpace(string(data))
}

// getCPUInfo gets CPU information
func getCPUInfo() (string, string) {
	// Get CPU info
	cpuInfo, err := cpu.Info()
	if err != nil || len(cpuInfo) == 0 {
		return "Unknown", "Unknown"
	}

	// Get physical core count
	physicalCores, err := cpu.Counts(false)
	if err != nil {
		physicalCores = 0
	}

	// Get CPU frequencies
	frequencies, err := cpu.Percent(time.Second, false)
	if err != nil || len(frequencies) == 0 {
		return cpuInfo[0].ModelName, "Unknown"
	}

	// Format CPU frequency
	freqStr := "Unknown"
	if cpuInfo[0].Mhz > 0 {
		freq := cpuInfo[0].Mhz
		if freq > 1000.0 {
			freqStr = fmt.Sprintf("%.2f GHz (%s)", freq/1000.0, translateCores(physicalCores))
		} else {
			freqStr = fmt.Sprintf("%.2f MHz (%s)", freq, translateCores(physicalCores))
		}
	}

	return cpuInfo[0].ModelName, freqStr
}

// translateCores translates the number of CPU cores into its corresponding name
func translateCores(num int) string {
	names := map[int]string{
		1:  "single-core",
		2:  "dual-core",
		4:  "quad-core",
		8:  "octa-core",
		16: "hexa-core",
		32: "deca-core",
		64: "dodeca-core",
	}

	if name, ok := names[num]; ok {
		return name
	}
	return fmt.Sprintf("%d-core", num)
}

// getMemoryInfo gets memory usage information
func getMemoryInfo() MemoryData {
	v, err := mem.VirtualMemory()
	if err != nil {
		return MemoryData{
			Formatted: "Unknown",
			Percent:   0,
		}
	}

	formatted := fmt.Sprintf("%s / %s (%.2f%%)",
		formatSize(v.Used), formatSize(v.Total), v.UsedPercent)

	return MemoryData{
		Formatted: formatted,
		Percent:   v.UsedPercent,
	}
}

// getDiskInfo gets disk usage information
func getDiskInfo() DiskData {
	usage, err := disk.Usage("/")
	if err != nil {
		return DiskData{
			Formatted: "Unknown",
			Percent:   0,
		}
	}

	formatted := fmt.Sprintf("%s / %s (%.2f%%)",
		formatSize(usage.Used), formatSize(usage.Total), usage.UsedPercent)

	return DiskData{
		Formatted: formatted,
		Percent:   usage.UsedPercent,
	}
}

// formatSize formats bytes to the appropriate size in KB, MB, GB, or TB
func formatSize(bytes uint64) string {
	kilobytes := float64(bytes) / 1024
	megabytes := kilobytes / 1024
	gigabytes := megabytes / 1024
	terabytes := gigabytes / 1024

	if gigabytes >= 1000 {
		return fmt.Sprintf("%.2f TB", terabytes)
	} else if megabytes >= 1000 {
		return fmt.Sprintf("%.2f GB", gigabytes)
	} else if kilobytes >= 1000 {
		return fmt.Sprintf("%.2f MB", megabytes)
	} else if kilobytes >= 1 {
		return fmt.Sprintf("%.2f KB", kilobytes)
	}
	return fmt.Sprintf("%d B", bytes)
}

// getCPUUsage gets CPU usage percentage
func getCPUUsage() (float64, error) {
	percentages, err := cpu.Percent(time.Second, false)
	if err != nil || len(percentages) == 0 {
		return 0, err
	}
	return percentages[0], nil
}

// getLoadAverage gets system load averages
func getLoadAverage() []string {
	avgStat, err := load.Avg()
	if err != nil {
		return []string{"0.00", "0.00", "0.00"}
	}

	return []string{
		fmt.Sprintf("%.2f", avgStat.Load1),
		fmt.Sprintf("%.2f", avgStat.Load5),
		fmt.Sprintf("%.2f", avgStat.Load15),
	}
}

// getCPUTemperature gets CPU temperature
func getCPUTemperature() string {
	// Try to read temperature from hwmon
	data, err := ioutil.ReadFile("/sys/class/hwmon/hwmon1/temp1_input")
	if err == nil {
		tempStr := strings.TrimSpace(string(data))
		tempInt, err := strconv.Atoi(tempStr)
		if err == nil {
			tempCelsius := float64(tempInt) / 1000.0
			return fmt.Sprintf("%.2f°C", tempCelsius)
		}
	}

	// If hwmon1 fails, try hwmon0
	data, err = ioutil.ReadFile("/sys/class/hwmon/hwmon0/temp1_input")
	if err == nil {
		tempStr := strings.TrimSpace(string(data))
		tempInt, err := strconv.Atoi(tempStr)
		if err == nil {
			tempCelsius := float64(tempInt) / 1000.0
			return fmt.Sprintf("%.2f°C", tempCelsius)
		}
	}

	return "N/A"
}

// getUptime gets system uptime
func getUptime() string {
	uptime, err := host.Uptime()
	if err != nil {
		return "Unknown"
	}

	// Convert to time.Duration for easier formatting
	uptimeDuration := time.Duration(uptime) * time.Second

	days := int(uptimeDuration.Hours() / 24)
	hours := int(uptimeDuration.Hours()) % 24
	minutes := int(uptimeDuration.Minutes()) % 60
	seconds := int(uptimeDuration.Seconds()) % 60

	return fmt.Sprintf("%d days %d hours %d mins %d secs", days, hours, minutes, seconds)
}
