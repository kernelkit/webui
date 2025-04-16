package api

import (
	"crypto/rand"
	"encoding/hex"
	"html/template"
	"net/http"
	"time"

	"github.com/gorilla/sessions"
)

// SessionStore is the global session store
var SessionStore *sessions.CookieStore

// SessionName is the name of the session cookie
const SessionName = "kernelkit"

// SessionLifetime is the session lifetime in seconds (10 minutes)
const SessionLifetime = 600

// InitSessionStore initializes the session store with a random key
func InitSessionStore() {
	// Generate a random secret key
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		panic("Failed to generate session key")
	}

	// Create the session store
	SessionStore = sessions.NewCookieStore(key)
	SessionStore.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   SessionLifetime,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteNoneMode,
	}
}

// GenerateSecretKey generates a random secret key
func GenerateSecretKey() string {
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		panic("Failed to generate secret key")
	}
	return hex.EncodeToString(key)
}

// LoginHandler handles user login
func LoginHandler(tmpl *template.Template) http.HandlerFunc {
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
			session, err := SessionStore.Get(r, SessionName)
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
				session, _ := SessionStore.Get(r, SessionName)
				session.AddFlash("Login failed! Incorrect username or password.")
				session.Save(r, w)
				http.Redirect(w, r, "/", http.StatusFound)
			}
		}
	}
}

// LogoutHandler handles user logout
func LogoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get session
		session, err := SessionStore.Get(r, SessionName)
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

// KeepAliveHandler keeps the session alive
func KeepAliveHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get session
		session, err := SessionStore.Get(r, SessionName)
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

// AuthMiddleware is a middleware that checks if the user is authenticated
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get session
		session, err := SessionStore.Get(r, SessionName)
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

// IndexHandler handles the index page
func IndexHandler(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get session
		session, err := SessionStore.Get(r, SessionName)
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
		// This would be implemented in a separate function in the actual code
		manualFiles := []string{"example1", "example2"}

		// Render main page
		tmpl.ExecuteTemplate(w, "main.html", map[string]interface{}{
			"Username":    username,
			"ManualFiles": manualFiles,
		})
	}
}
