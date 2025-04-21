package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type LogInfo struct {
	Files     []string
	ActiveLog string
	Content   string
}

// logHandler handles the log viewing page
func logHandler(w http.ResponseWriter, r *http.Request) {
	// Get list of log files
	files, err := listLogFiles()
	if err != nil {
		log.Printf("Error listing log files: %v", err)
		http.Error(w, "Failed to read log directory", http.StatusInternalServerError)
		return
	}

	// Default display - no file selected yet
	info := &LogInfo{
		Files:     files,
		ActiveLog: "",
		Content:   "",
	}

	// Check if a specific log file is requested
	if logFile := r.URL.Query().Get("file"); logFile != "" {
		// Validate the filename to prevent directory traversal
		if strings.Contains(logFile, "..") || strings.Contains(logFile, "/") {
			http.Error(w, "Invalid log filename", http.StatusBadRequest)
			return
		}

		content, err := readLogFile(logFile)
		if err != nil {
			log.Printf("Error reading log file %s: %v", logFile, err)
			http.Error(w, fmt.Sprintf("Failed to read log file: %v", err), http.StatusInternalServerError)
			return
		}

		info.ActiveLog = logFile
		info.Content = content
	}

	renderPage(w, r, "log", info)
}

// listLogFiles returns a sorted list of log files from /var/log
func listLogFiles() ([]string, error) {
	// Read the log directory
	files, err := os.ReadDir("/var/log")
	if err != nil {
		return nil, err
	}

	// Filter and sort log files
	var logs []string
	for _, file := range files {
		if !file.IsDir() {
			logs = append(logs, file.Name())
		}
	}
	sort.Strings(logs)

	return logs, nil
}

// readLogFile reads the content of a log file, handling gzipped files
func readLogFile(filename string) (string, error) {
	// Construct the full file path
	filePath := filepath.Join("/var/log", filename)

	// Check if the file exists
	_, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("log file not found")
	}

	// Check if it's a large file (>1MB) and use tail instead
	fileInfo, err := os.Stat(filePath)
	if err == nil && fileInfo.Size() > 1024*1024 {
		return tailLogFile(filename, "1000")
	}

	// Check if the file is gzipped
	if strings.HasSuffix(filename, ".gz") {
		// Open the gzipped file
		file, err := os.Open(filePath)
		if err != nil {
			return "", err
		}
		defer file.Close()

		// Create a gzip reader
		gzipReader, err := gzip.NewReader(file)
		if err != nil {
			return "", err
		}
		defer gzipReader.Close()

		// Read the content
		content, err := io.ReadAll(gzipReader)
		if err != nil {
			return "", err
		}

		return string(content), nil
	}

	// For non-gzipped files, read directly
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// tailLogFile executes the tail command on a log file
func tailLogFile(filename string, lines string) (string, error) {
	if lines == "" {
		lines = "100" // Default to 100 lines
	}

	// Construct the full file path
	filePath := filepath.Join("/var/log", filename)

	// Execute the tail command
	cmd := exec.Command("tail", "-n", lines, filePath)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return string(output), nil
}

// tailLogHandler handles AJAX requests for tailing logs
func tailLogHandler(w http.ResponseWriter, r *http.Request) {
	// Get log filename from query parameter
	filename := r.URL.Query().Get("file")
	if filename == "" {
		http.Error(w, "Log filename is required", http.StatusBadRequest)
		return
	}

	// Validate the filename to prevent directory traversal
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") {
		http.Error(w, "Invalid log filename", http.StatusBadRequest)
		return
	}

	// Get the number of lines to tail
	lines := r.URL.Query().Get("lines")

	// Tail the log file
	content, err := tailLogFile(filename, lines)
	if err != nil {
		log.Printf("Error tailing log file %s: %v", filename, err)
		http.Error(w, fmt.Sprintf("Failed to tail log file: %v", err), http.StatusInternalServerError)
		return
	}

	// Set content type and write the output
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(content))
}
