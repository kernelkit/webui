package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/spf13/pflag"
)

//go:embed assets
var assetFS embed.FS

//go:embed templates
var templateFS embed.FS

var (
	sessionSecret string
	sessionPath   string
)

var templates map[string]*template.Template

type SessionConfig struct {
	Secret string `json:"secret"`
}

type PageData struct {
	Title       string
	Username    string
	Content     interface{}
	ManualFiles []string
}

// Command line options
var (
	port       int
	debug      bool
	secretPath string
)

func main() {
	pflag.IntVarP(&port, "port", "p", 8080, "HTTP listening port, default: 8080")
	pflag.BoolVarP(&debug, "debug", "d", false, "Enable debug mode (allows admin/admin login)")
	pflag.StringVarP(&sessionPath, "secret", "s", "/var/lib/misc/", "Directory for session secret")
	pflag.Parse()

	if debug {
		log.Println("WARNING: Debug mode enabled - insecure authentication is active")
	}

	if err := ensureSessionSecret(); err != nil {
		log.Fatal("Failed to secure session secret:", err)
	}

	if err := loadTemplates(); err != nil {
		log.Fatal("Failed to load templates:", err)
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Serve static files (for favicon.ico and other assets)
	fileServer := http.FileServer(http.FS(assetFS))
	r.Handle("/assets/*", http.StripPrefix("/", fileServer))

	// Public routes
	r.Group(func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
		})
		r.Get("/login", loginPageHandler)
		r.Post("/login", loginHandler)
	})

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware)
		r.Get("/logout", logoutHandler)
		r.Get("/status", statusHandler)
		r.Get("/manual", manualHandler)
		r.Get("/manual/{name}", manualHandler)
		r.Get("/network", networkHandler)
		r.Get("/log", logHandler)
		r.Get("/tail-log", tailLogHandler)
	})

	// Only localhost, use nginx or similar to access
	listenAddr := fmt.Sprintf("localhost:%d", port)
	log.Printf("Server starting at http://%s\n", listenAddr)
	log.Fatal(http.ListenAndServe(listenAddr, r))
}

func loadTemplates() error {
	templates = make(map[string]*template.Template)

	tmplFiles, err := templateFS.ReadDir("templates")
	if err != nil {
		return err
	}

	layoutContent, err := templateFS.ReadFile("templates/layout.html")
	if err != nil {
		return err
	}

	for _, file := range tmplFiles {
		var tmpl *template.Template

		if file.IsDir() {
			continue
		}

		fileName := file.Name()
		name := strings.TrimSuffix(fileName, ".html")

		// Skip the layout template itself
		if name == "layout" {
			continue
		}

		content, err := templateFS.ReadFile("templates/" + fileName)
		if err != nil {
			return err
		}

		isStandalone := strings.Contains(string(content), "<!DOCTYPE html>") ||
			strings.Contains(string(content), "<html")

		if isStandalone {
			// Standalone template (like login)
			tmpl = template.New(fileName)
			tmpl, err = tmpl.Parse(string(content))
		} else {
			// Template that uses layout
			tmpl = template.New("layout.html")
			tmpl, err = tmpl.Parse(string(layoutContent))
			if err != nil {
				return err
			}
			tmpl, err = tmpl.Parse(string(content))
		}

		if err != nil {
			return err
		}

		templates[name] = tmpl
	}

	return nil
}

func renderPage(w http.ResponseWriter, r *http.Request, nm string, info interface{}) {
	var err error

	tmpl, ok := templates[nm]
	if !ok {
		http.NotFound(w, r)
		return
	}

	manualFiles, err := listManualFiles()
	if err != nil {
		log.Printf("Error listing manual files: %v", err)
		manualFiles = []string{}
	}

	// Build common page data
	data := PageData{
		Title:       strings.Title(nm),
		Username:    getUsername(r),
		Content:     info,
		ManualFiles: manualFiles,
	}

	if r.Header.Get("HX-Request") == "true" {
		err = tmpl.ExecuteTemplate(w, "content", info)
	} else {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		err = tmpl.ExecuteTemplate(w, "layout.html", data)
	}

	if err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func loginPageHandler(w http.ResponseWriter, r *http.Request) {
	// Check if already logged in
	if _, err := r.Cookie("session"); err == nil {
		http.Redirect(w, r, "/status", http.StatusSeeOther)
		return
	}

	// Get the login template
	tmpl, ok := templates["login"]
	if !ok {
		http.Error(w, "Login template not found", http.StatusInternalServerError)
		return
	}

	// Prepare data with error message if present
	errorMsg := ""
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		switch errParam {
		case "empty_fields":
			errorMsg = "Username and password are required"
		case "invalid_credentials":
			errorMsg = "Invalid username or password"
		default:
			errorMsg = "An error occurred during login"
		}
	}

	data := map[string]interface{}{
		"ErrorMessage": errorMsg,
	}

	// For login page, we execute the template directly (not via layout)
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	username := r.Form.Get("username")
	password := r.Form.Get("password")

	if username == "" || password == "" {
		http.Redirect(w, r, "/login?error=empty_fields", http.StatusSeeOther)
		return
	}

	// Authenticate user
	authenticated := false

	// In debug mode, allow admin/admin
	if debug && username == "admin" && password == "admin" {
		authenticated = true
	} else {
		// Use PAM for authentication
		authenticated = authenticateWithPAM(username, password)
	}

	if !authenticated {
		http.Redirect(w, r, "/login?error=invalid_credentials", http.StatusSeeOther)
		return
	}

	// Set session cookie
	expiration := time.Now().Add(24 * time.Hour)
	cookie := http.Cookie{
		Name:     "session",
		Value:    createSessionToken(username),
		Expires:  expiration,
		HttpOnly: true,
		Path:     "/",
	}
	http.SetCookie(w, &cookie)

	/*
	 * For HTMX requests, i.e., from login.html page,
	 * force a full page refresh
	 */
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/status")
		return
	}

	http.Redirect(w, r, "/status", http.StatusSeeOther)
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	// Clear the session cookie
	cookie := http.Cookie{
		Name:     "session",
		Value:    "",
		MaxAge:   -1,
		HttpOnly: true,
		Path:     "/",
	}
	http.SetCookie(w, &cookie)

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")

		if err != nil || cookie.Value == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Validate session token (in a real app, you'd check a session store)
		username := validateSessionToken(cookie.Value)
		if username == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Store username in request context for use in handlers
		ctx := r.Context()
		ctx = context.WithValue(ctx, "username", username)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func getUsername(r *http.Request) string {
	if username, ok := r.Context().Value("username").(string); ok {
		return username
	}
	return ""
}

// For a real application, use a proper session management library
func createSessionToken(username string) string {
	// This is a simplified example - use a proper session library in production
	return username + "-" + sessionSecret
}

func validateSessionToken(token string) string {
	// Simple validation for demonstration purposes
	if parts := strings.Split(token, "-"); len(parts) >= 2 && parts[1] == sessionSecret {
		return parts[0]
	}
	return ""
}

// ensureSessionSecret makes sure we have a valid session secret
// It will try to load one from disk, or generate a new one if needed
func ensureSessionSecret() error {
	// If provided on command line, use that
	if sessionSecret != "" {
		return nil
	}

	configPath := filepath.Join(sessionPath, "session.json")

	// Try to load existing config
	data, err := os.ReadFile(configPath)
	if err == nil {
		var config SessionConfig
		if err := json.Unmarshal(data, &config); err == nil && config.Secret != "" {
			sessionSecret = config.Secret
			log.Println("Using existing session secret from", configPath)
			return nil
		}
	}

	// Generate a new secret
	secret := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, secret); err != nil {
		return err
	}
	sessionSecret = base64.StdEncoding.EncodeToString(secret)

	// Save to disk
	config := SessionConfig{Secret: sessionSecret}
	data, err = json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return err
	}

	log.Println("Generated and saved new session secret to", configPath)
	return nil
}
