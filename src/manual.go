package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

// ManualInfo holds data for rendering manual pages
type ManualInfo struct {
	Title   string
	Content template.HTML
}

// manualHandler handles requests for manual pages
func manualHandler(w http.ResponseWriter, r *http.Request) {
	// Get manual files list
	files, err := listManualFiles()
	if err != nil {
		log.Printf("Error listing manual files: %v", err)
		http.Error(w, "Failed to list manual files", http.StatusInternalServerError)
		return
	}

	// Check if we have a specific manual page requested
	name := chi.URLParam(r, "name")

	// If no specific page is requested, show the list
	if name == "" {
		renderPage(w, r, "manual-list", map[string]interface{}{
			"Files": files,
		})
		return
	}

	// Validate the filename to prevent directory traversal
	if strings.Contains(name, "..") || strings.Contains(name, "/") {
		http.Error(w, "Invalid manual filename", http.StatusBadRequest)
		return
	}

	// Ensure the filename has .md extension
	if !strings.HasSuffix(name, ".md") {
		name = name + ".md"
	}

	// Construct the full file path
	filePath := filepath.Join("./manual", name)

	// Check if the file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.Error(w, "Manual page not found", http.StatusNotFound)
		return
	}

	// Read the markdown file
	content, err := os.ReadFile(filePath)
	if err != nil {
		log.Printf("Error reading manual file %s: %v", name, err)
		http.Error(w, "Failed to read manual page", http.StatusInternalServerError)
		return
	}

	// Convert markdown to HTML
	htmlContent := mdToHTML(content)

	// Create a title from the filename
	title := strings.TrimSuffix(name, ".md")
	title = strings.ReplaceAll(title, "-", " ")
	title = strings.ReplaceAll(title, "_", " ")
	title = strings.Title(title) // Capitalize first letter of each word

	// Render the manual page
	renderPage(w, r, "manual", &ManualInfo{
		Title:   title,
		Content: template.HTML(htmlContent),
	})
}

// listManualFiles returns a list of all manual files
func listManualFiles() ([]string, error) {
	// Read files from the manual directory
	manualDir := "./manual" // Adjust as needed
	if _, err := os.Stat(manualDir); os.IsNotExist(err) {
		// If the directory doesn't exist, create it
		// if err := os.MkdirAll(manualDir, 0755); err != nil {
		// 	return nil, err
		// }
		return []string{}, nil
	}

	files, err := os.ReadDir(manualDir)
	if err != nil {
		return nil, err
	}

	// Filter for markdown files
	var manualFiles []string
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".md") {
			// Store just the filename without extension
			baseName := strings.TrimSuffix(file.Name(), ".md")
			manualFiles = append(manualFiles, baseName)
		}
	}

	return manualFiles, nil
}

// mdToHTML converts markdown content to HTML
func mdToHTML(md []byte) []byte {
	// Create markdown parser with extensions
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse(md)

	// Create HTML renderer with extensions
	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	return markdown.Render(doc, renderer)
}
