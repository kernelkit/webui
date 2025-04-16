package main

import (
	"compress/gzip"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
)

// Command line flags
var (
	port         = flag.Int("port", 8080, "HTTP port to listen on")
	debug        = flag.Bool("debug", false, "Enable debug mode")
	assetsDir    = flag.String("assets", "./assets", "Path to static assets")
	templatesDir = flag.String("templates", "./templates", "Path to HTML templates")
	tlsCert      = flag.String("tls-cert", "", "Path to TLS certificate")
	tlsKey       = flag.String("tls-key", "", "Path to TLS key")
)

// Global session store
var sessionStore *sessions.CookieStore

// SessionName is the name of the session cookie
const SessionName = "infix-session"

// SessionLifetime is the session lifetime in seconds (10 minutes)
const SessionLifetime = 600

// loadTemplates loads all templates from the specified directory
func loadTemplates(dir string) (*template.Template, error) {
	tmpl := template.New("")

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Only process HTML files
		if filepath.Ext(path) != ".html" {
			return nil
		}

		// Read template file
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// Get the relative path from the templates directory
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		// Use the relative path as the template name
		_, err = tmpl.New(rel).Parse(string(content))
		return err
	})

	return tmpl, err
}

// initSessionStore initializes the session store with a random key
func initSessionStore() {
	// Generate a random secret key
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		// If reading from stdin fails, use a static key (less secure but works for testing)
		key = []byte("infix-webui-secret-key-for-development-only")
	}

	// Create the session store
	sessionStore = sessions.NewCookieStore(key)
	sessionStore.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   SessionLifetime,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteNoneMode,
	}
}

// authMiddleware checks if the user is authenticated
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get session
		session, err := sessionStore.Get(r, SessionName)
		if err != nil {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		// Check if user is logged in
		if auth, ok := session.Values["logged_in"].(bool); !ok || !auth {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		// Update session expiration
		session.Options.MaxAge = SessionLifetime
		session.Save(r, w)

		// Call the next handler
		next.ServeHTTP(w, r)
	})
}

// indexHandler handles the index page
func indexHandler(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get session
		session, err := sessionStore.Get(r, SessionName)
		if err != nil {
			tmpl.ExecuteTemplate(w, "login.html", nil)
			return
		}

		// Check if user is logged in
		if auth, ok := session.Values["logged_in"].(bool); !ok || !auth {
			// Check for flash messages
			var flashMessages []string
			if flashes := session.Flashes(); len(flashes) > 0 {
				for _, flash := range flashes {
					if msg, ok := flash.(string); ok {
						flashMessages = append(flashMessages, msg)
					}
				}
				session.Save(r, w)
			}

			tmpl.ExecuteTemplate(w, "login.html", map[string]interface{}{
				"Flashes": flashMessages,
			})
			return
		}

		// User is logged in, get username
		username, _ := session.Values["username"].(string)

		// Find manual files
		manualDir := filepath.Join(*assetsDir, "manual")
		var manualFiles []string

		// Check if manual directory exists
		if _, err := os.Stat(manualDir); err == nil {
			// Read manual files
			files, err := os.ReadDir(manualDir)
			if err == nil {
				for _, file := range files {
					if !file.IsDir() && filepath.Ext(file.Name()) == ".gz" {
						// Remove the .html.gz extension
						name := file.Name()
						name = name[:len(name)-8] // Remove .html.gz
						manualFiles = append(manualFiles, name)
					}
				}
			}
		}

		// Render main page
		tmpl.ExecuteTemplate(w, "main.html", map[string]interface{}{
			"Username":    username,
			"ManualFiles": manualFiles,
		})
	}
}

// loginHandler handles user login
func loginHandler(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only POST requests are allowed
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse form data
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}

		// Get credentials from form
		username := r.Form.Get("username")
		password := r.Form.Get("password")

		// Check credentials (hardcoded for now, just like in the Flask app)
		if username == "admin" && password == "admin" {
			// Get session
			session, err := sessionStore.Get(r, SessionName)
			if err != nil {
				http.Error(w, "Failed to get session", http.StatusInternalServerError)
				return
			}

			// Set session values
			session.Values["logged_in"] = true
			session.Values["username"] = username
			session.Options.MaxAge = SessionLifetime

			// Save session
			if err := session.Save(r, w); err != nil {
				http.Error(w, "Failed to save session", http.StatusInternalServerError)
				return
			}

			// Redirect to index
			http.Redirect(w, r, "/", http.StatusFound)
		} else {
			// If HTMX is used, return a specific fragment
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Retarget", "#login-form")
				w.Header().Set("HX-Reswap", "outerHTML")
				tmpl.ExecuteTemplate(w, "login_form.html", map[string]interface{}{
					"Error": "Login failed! Incorrect username or password.",
				})
			} else {
				// Otherwise redirect to index with error in session
				session, _ := sessionStore.Get(r, SessionName)
				session.AddFlash("Login failed! Incorrect username or password.")
				session.Save(r, w)
				http.Redirect(w, r, "/", http.StatusFound)
			}
		}
	}
}

// logoutHandler handles user logout
func logoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get session
		session, err := sessionStore.Get(r, SessionName)
		if err != nil {
			http.Error(w, "Failed to get session", http.StatusInternalServerError)
			return
		}

		// Clear session
		session.Values = make(map[interface{}]interface{})
		session.Options.MaxAge = -1

		// Save session
		if err := session.Save(r, w); err != nil {
			http.Error(w, "Failed to save session", http.StatusInternalServerError)
			return
		}

		// Redirect to index
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// keepAliveHandler keeps the session alive
func keepAliveHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get session
		session, err := sessionStore.Get(r, SessionName)
		if err != nil {
			http.Error(w, "Failed to get session", http.StatusInternalServerError)
			return
		}

		// Check if user is logged in
		if auth, ok := session.Values["logged_in"].(bool); !ok || !auth {
			http.Error(w, "Not logged in", http.StatusUnauthorized)
			return
		}

		// Reset the expiration time
		session.Options.MaxAge = SessionLifetime

		// Save session
		if err := session.Save(r, w); err != nil {
			http.Error(w, "Failed to save session", http.StatusInternalServerError)
			return
		}

		// Return success
		w.Write([]byte("Session is kept alive"))
	}
}

// setupRoutes configures all the application routes
func setupRoutes(r *mux.Router, tmpl *template.Template) {
	// Serve static files
	r.PathPrefix("/assets/").Handler(http.StripPrefix("/assets/", http.FileServer(http.Dir(*assetsDir))))

	// Public routes (no authentication required)
	r.HandleFunc("/", indexHandler(tmpl)).Methods("GET")
	r.HandleFunc("/login", loginHandler(tmpl)).Methods("POST")
	r.HandleFunc("/keepalive", keepAliveHandler()).Methods("GET")

	// Create a subrouter for protected routes
	protected := r.PathPrefix("/").Subrouter()
	protected.Use(authMiddleware)

	// Main application pages
	protected.HandleFunc("/logout", logoutHandler()).Methods("GET")
	protected.HandleFunc("/status", statusHandler(tmpl)).Methods("GET")
	protected.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		tmpl.ExecuteTemplate(w, "config.html", nil)
	}).Methods("GET")
	protected.HandleFunc("/log", logsHandler(tmpl)).Methods("GET")
	protected.HandleFunc("/net", networkHandler(tmpl)).Methods("GET")
	protected.HandleFunc("/upgrade", func(w http.ResponseWriter, r *http.Request) {
		tmpl.ExecuteTemplate(w, "upgrade.html", nil)
	}).Methods("GET")

	// API routes
	api := protected.PathPrefix("/api").Subrouter()

	// Status API
	api.HandleFunc("/status/refresh", statusHandler(tmpl)).Methods("GET")

	// Network API
	api.HandleFunc("/network/interfaces", networkHandler(tmpl)).Methods("GET")
	api.HandleFunc("/network/interface/{name}", networkInterfaceHandler(tmpl)).Methods("GET")
	api.HandleFunc("/network/interface/{name}", networkInterfaceUpdateHandler(tmpl)).Methods("POST")

	// Logs API
	api.HandleFunc("/logs", listLogsHandler()).Methods("GET")
	api.HandleFunc("/logs/{filename}", getLogHandler()).Methods("GET")
	api.HandleFunc("/tail-log/{filename}", tailLogHandler()).Methods("GET")

	// Upload API
	api.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseMultipartForm(150 << 20) // 150 MB limit
		if err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}

		file, handler, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "No file found", http.StatusBadRequest)
			return
		}
		defer file.Close()

		tempFile, err := os.Create(filepath.Join("/tmp", handler.Filename))
		if err != nil {
			http.Error(w, "Failed to save file", http.StatusInternalServerError)
			return
		}
		defer tempFile.Close()

		_, err = io.Copy(tempFile, file)
		if err != nil {
			http.Error(w, "Failed to save file", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"next": "progress"})
	}).Methods("POST")

	// Manual pages
	protected.HandleFunc("/manual/{page}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		pageName := vars["page"]

		// Path to manual files
		manualDir := filepath.Join(*assetsDir, "manual")
		htmlGzFile := filepath.Join(manualDir, pageName+".html.gz")

		// Check if file exists
		if _, err := os.Stat(htmlGzFile); os.IsNotExist(err) {
			http.Error(w, "Page not found", http.StatusNotFound)
			return
		}

		// Read and decompress file
		f, err := os.Open(htmlGzFile)
		if err != nil {
			http.Error(w, "Failed to open manual page", http.StatusInternalServerError)
			return
		}
		defer f.Close()

		gzReader, err := gzip.NewReader(f)
		if err != nil {
			http.Error(w, "Failed to decompress manual page", http.StatusInternalServerError)
			return
		}
		defer gzReader.Close()

		content, err := io.ReadAll(gzReader)
		if err != nil {
			http.Error(w, "Failed to read manual page", http.StatusInternalServerError)
			return
		}

		// Render template
		tmpl.ExecuteTemplate(w, "manual.html", map[string]interface{}{
			"Content": string(content),
			"Page":    pageName,
		})
	}).Methods("GET")
}

// statusHandler and other handler function stubs that would be implemented in the respective files
func statusHandler(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// This would be implemented in status.go
		// For now, just render a placeholder template
		tmpl.ExecuteTemplate(w, "status.html", nil)
	}
}

func networkHandler(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// This would be implemented in network.go
		// For now, just render a placeholder template
		tmpl.ExecuteTemplate(w, "net.html", nil)
	}
}

func networkInterfaceHandler(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// This would be implemented in network.go
		vars := mux.Vars(r)
		name := vars["name"]

		// Return a placeholder JSON response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"name":   name,
			"status": "up",
		})
	}
}

func networkInterfaceUpdateHandler(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// This would be implemented in network.go
		vars := mux.Vars(r)
		name := vars["name"]

		// Return a placeholder success message
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "success",
			"message": fmt.Sprintf("Updated interface %s", name),
		})
	}
}

func logsHandler(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// This would be implemented in logs.go
		// For now, just render a placeholder template
		tmpl.ExecuteTemplate(w, "log.html", nil)
	}
}

func listLogsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// This would be implemented in logs.go
		// Return a placeholder log list
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]string{"syslog", "auth.log", "kern.log"})
	}
}

func getLogHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// This would be implemented in logs.go
		vars := mux.Vars(r)
		filename := vars["filename"]

		// Return a placeholder message
		w.Write([]byte(fmt.Sprintf("Contents of %s would be shown here", filename)))
	}
}

func tailLogHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// This would be implemented in logs.go
		vars := mux.Vars(r)
		filename := vars["filename"]

		// Return a placeholder message
		w.Write([]byte(fmt.Sprintf("Tail of %s would be shown here", filename)))
	}
}

func main() {
	// Parse command line flags
	flag.Parse()

	// Configure logging
	if *debug {
		log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
		log.Println("Debug mode enabled")
	} else {
		log.SetFlags(log.Ldate | log.Ltime)
	}

	// Initialize session store
	initSessionStore()

	// Load templates
	log.Printf("Loading templates from %s", *templatesDir)
	tmpl, err := loadTemplates(*templatesDir)
	if err != nil {
		log.Fatalf("Failed to load templates: %v", err)
	}

	// Create router
	r := mux.NewRouter()

	// Apply middleware
	// CORS middleware
	corsMiddleware := handlers.CORS(
		handlers.AllowedOrigins([]string{"*"}),
		handlers.AllowedMethods([]string{"GET", "POST", "OPTIONS"}),
		handlers.AllowedHeaders([]string{"Content-Type", "Authorization"}),
	)

	// Logging middleware
	loggingMiddleware := handlers.LoggingHandler(os.Stdout, r)

	// Recovery middleware
	recoveryMiddleware := handlers.RecoveryHandler()(r)

	// Combine middleware
	handler := corsMiddleware(loggingMiddleware)
	handler = recoveryMiddleware

	// Set up routes
	setupRoutes(r, tmpl)

	// Start the server
	serverAddr := fmt.Sprintf(":%d", *port)
	log.Printf("Starting server on %s", serverAddr)

	// Use TLS if certificate and key are provided
	if *tlsCert != "" && *tlsKey != "" {
		log.Printf("Using TLS with certificate %s and key %s", *tlsCert, *tlsKey)
		log.Fatal(http.ListenAndServeTLS(serverAddr, *tlsCert, *tlsKey, handler))
	} else {
		log.Printf("TLS not configured, running without HTTPS")
		log.Fatal(http.ListenAndServe(serverAddr, handler))
	}
}
