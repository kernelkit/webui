package main

import (
	"compress/gzip"
	"encoding/json"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// LogsHandler handles the log viewing page
func LogsHandler(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Render the logs page
		tmpl.ExecuteTemplate(w, "log.html", nil)
	}
}

// ListLogsHandler lists all log files in /var/log
func ListLogsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Read the log directory
		files, err := ioutil.ReadDir("/var/log")
		if err != nil {
			http.Error(w, "Failed to read log directory", http.StatusInternalServerError)
			return
		}

		// Filter and sort log files
		var logs []string
		for _, file := range files {
			if !file.IsDir() {
				logs = append(logs, file.Name())
			}
		}
		sort.Strings(logs)

		// Return the list of logs
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(logs)
	}
}

// GetLogHandler retrieves and displays a specific log file
func GetLogHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get log filename from URL path
		filename := strings.TrimPrefix(r.URL.Path, "/logs/")
		if filename == "" {
			http.Error(w, "Log filename is required", http.StatusBadRequest)
			return
		}

		// Validate the filename to prevent directory traversal
		if strings.Contains(filename, "..") || strings.Contains(filename, "/") {
			http.Error(w, "Invalid log filename", http.StatusBadRequest)
			return
		}

		// Construct the full file path
		filePath := filepath.Join("/var/log", filename)

		// Check if the file exists
		_, err := os.Stat(filePath)
		if os.IsNotExist(err) {
			http.Error(w, "Log file not found", http.StatusNotFound)
			return
		}

		// Check if the file is gzipped
		if strings.HasSuffix(filename, ".gz") {
			// Open the gzipped file
			file, err := os.Open(filePath)
			if err != nil {
				http.Error(w, "Failed to open log file", http.StatusInternalServerError)
				return
			}
			defer file.Close()

			// Create a gzip reader
			gzipReader, err := gzip.NewReader(file)
			if err != nil {
				http.Error(w, "Failed to read gzipped log file", http.StatusInternalServerError)
				return
			}
			defer gzipReader.Close()

			// Read the content
			content, err := io.ReadAll(gzipReader)
			if err != nil {
				http.Error(w, "Failed to read log content", http.StatusInternalServerError)
				return
			}

			// Set content type and write the content
			w.Header().Set("Content-Type", "text/plain")
			w.Write(content)
			return
		}

		// For non-gzipped files, serve the file directly
		http.ServeFile(w, r, filePath)
	}
}

// StreamLogHandler creates a streaming endpoint for log files
// This allows real-time viewing of logs with automatic updates
func StreamLogHandler(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get log filename from URL path
		filename := strings.TrimPrefix(r.URL.Path, "/stream-log/")
		if filename == "" {
			http.Error(w, "Log filename is required", http.StatusBadRequest)
			return
		}

		// Validate the filename to prevent directory traversal
		if strings.Contains(filename, "..") || strings.Contains(filename, "/") {
			http.Error(w, "Invalid log filename", http.StatusBadRequest)
			return
		}

		// Set up Server-Sent Events
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		// Flush the headers to send them to the client
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		// Render template for streaming view
		tmpl.ExecuteTemplate(w, "log_stream.html", map[string]interface{}{
			"Filename": filename,
		})
	}
}

// TailLogHandler provides the tail functionality for logs
func TailLogHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get log filename from URL path
		filename := strings.TrimPrefix(r.URL.Path, "/tail-log/")
		if filename == "" {
			http.Error(w, "Log filename is required", http.StatusBadRequest)
			return
		}

		// Validate the filename to prevent directory traversal
		if strings.Contains(filename, "..") || strings.Contains(filename, "/") {
			http.Error(w, "Invalid log filename", http.StatusBadRequest)
			return
		}

		// Construct the full file path
		filePath := filepath.Join("/var/log", filename)

		// Check if the file exists
		_, err := os.Stat(filePath)
		if os.IsNotExist(err) {
			http.Error(w, "Log file not found", http.StatusNotFound)
			return
		}

		// Get the number of lines to tail
		lines := r.URL.Query().Get("lines")
		if lines == "" {
			lines = "100" // Default to 100 lines
		}

		// Execute the tail command
		cmd := exec.Command("tail", "-n", lines, filePath)
		output, err := cmd.Output()
		if err != nil {
			http.Error(w, "Failed to tail log file", http.StatusInternalServerError)
			return
		}

		// Set content type and write the output
		w.Header().Set("Content-Type", "text/plain")
		w.Write(output)
	}
}
