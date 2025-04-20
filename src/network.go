package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

// NetworkInterface represents a network interface
type NetworkInterface struct {
	Name      string   `json:"ifname"`
	State     string   `json:"operstate,omitempty"`
	Addresses []string `json:"addr_info,omitempty"`
}

// Route represents a network route
type Route struct {
	Destination string `json:"dst"`
	Gateway     string `json:"gateway,omitempty"`
	Device      string `json:"dev"`
	Protocol    string `json:"protocol,omitempty"`
}

// NetworkHandler handles the network page
func NetworkHandler(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get network interfaces
		interfaces, err := getNetworkInterfaces()
		if err != nil {
			http.Error(w, "Failed to get network interfaces", http.StatusInternalServerError)
			return
		}

		// Get IPv4 routes
		routes4, err := getNetworkRoutes(false)
		if err != nil {
			http.Error(w, "Failed to get IPv4 routes", http.StatusInternalServerError)
			return
		}

		// Get IPv6 routes
		routes6, err := getNetworkRoutes(true)
		if err != nil {
			http.Error(w, "Failed to get IPv6 routes", http.StatusInternalServerError)
			return
		}

		// Prepare data for template
		data := map[string]interface{}{
			"Interfaces": interfaces,
			"Routes4":    routes4,
			"Routes6":    routes6,
		}

		// Render template based on request type
		if r.Header.Get("HX-Request") == "true" {
			// Render just the content for HTMX
			tmpl.ExecuteTemplate(w, "net_content.html", data)
		} else {
			// Render full page
			tmpl.ExecuteTemplate(w, "net.html", data)
		}
	}
}

// getNetworkInterfaces gets network interfaces using the ip command
func getNetworkInterfaces() ([]map[string]interface{}, error) {
	// Execute ip -j addr command
	cmd := exec.Command("ip", "-j", "addr")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute ip command: %w", err)
	}

	// Parse JSON output
	var interfaces []map[string]interface{}
	if err := json.Unmarshal(output, &interfaces); err != nil {
		return nil, fmt.Errorf("failed to parse ip command output: %w", err)
	}

	// Sort interfaces
	sort.Slice(interfaces, func(i, j int) bool {
		return sortInterfaceNames(
			interfaces[i]["ifname"].(string),
			interfaces[j]["ifname"].(string),
		)
	})

	return interfaces, nil
}

// getNetworkRoutes gets network routes using the ip command
func getNetworkRoutes(ipv6 bool) ([]map[string]interface{}, error) {
	var cmd *exec.Cmd
	if ipv6 {
		cmd = exec.Command("ip", "-j", "-6", "route")
	} else {
		cmd = exec.Command("ip", "-j", "route")
	}

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute ip command: %w", err)
	}

	// Parse JSON output
	var routes []map[string]interface{}
	if err := json.Unmarshal(output, &routes); err != nil {
		return nil, fmt.Errorf("failed to parse ip command output: %w", err)
	}

	return routes, nil
}

// sortInterfaceNames compares two interface names for sorting
func sortInterfaceNames(a, b string) bool {
	// Put lo first
	if a == "lo" {
		return true
	}
	if b == "lo" {
		return false
	}

	// Split the interface names into parts
	aParts := splitInterfaceName(a)
	bParts := splitInterfaceName(b)

	// Compare parts
	for i := 0; i < len(aParts) && i < len(bParts); i++ {
		// If both parts are numeric, compare as numbers
		aNum, aErr := tryParseInt(aParts[i])
		bNum, bErr := tryParseInt(bParts[i])

		if aErr == nil && bErr == nil {
			// Both are numbers
			if aNum != bNum {
				return aNum < bNum
			}
		} else if aErr == nil {
			// a is a number, b is not
			return true
		} else if bErr == nil {
			// b is a number, a is not
			return false
		} else {
			// Both are strings, compare lexicographically
			if aParts[i] != bParts[i] {
				return aParts[i] < bParts[i]
			}
		}
	}

	// If all compared parts are equal, the shorter name comes first
	return len(aParts) < len(bParts)
}

// splitInterfaceName splits an interface name into parts
func splitInterfaceName(name string) []string {
	re := regexp.MustCompile(`([0-9]+)`)
	parts := re.FindAllStringIndex(name, -1)

	if len(parts) == 0 {
		return []string{name}
	}

	var result []string
	lastEnd := 0

	for _, part := range parts {
		start, end := part[0], part[1]

		// Add non-numeric part if exists
		if start > lastEnd {
			result = append(result, name[lastEnd:start])
		}

		// Add numeric part
		result = append(result, name[start:end])
		lastEnd = end
	}

	// Add remaining part if exists
	if lastEnd < len(name) {
		result = append(result, name[lastEnd:])
	}

	return result
}

// tryParseInt tries to parse a string as an integer
func tryParseInt(s string) (int, error) {
	var num int
	_, err := fmt.Sscanf(s, "%d", &num)
	return num, err
}

// NetworkInterfaceHandler handles specific network interface details
func NetworkInterfaceHandler(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get interface name from URL
		name := strings.TrimPrefix(r.URL.Path, "/api/network/interface/")
		if name == "" {
			http.Error(w, "Interface name is required", http.StatusBadRequest)
			return
		}

		// Get interface details
		cmd := exec.Command("ip", "-j", "addr", "show", name)
		output, err := cmd.Output()
		if err != nil {
			http.Error(w, "Failed to get interface details", http.StatusInternalServerError)
			return
		}

		// Parse JSON output
		var interfaces []map[string]interface{}
		if err := json.Unmarshal(output, &interfaces); err != nil {
			http.Error(w, "Failed to parse interface details", http.StatusInternalServerError)
			return
		}

		if len(interfaces) == 0 {
			http.Error(w, "Interface not found", http.StatusNotFound)
			return
		}

		// Get the interface details
		interfaceDetails := interfaces[0]

		// Render template or return JSON based on request
		if r.Header.Get("HX-Request") == "true" {
			tmpl.ExecuteTemplate(w, "interface_details.html", interfaceDetails)
		} else {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(interfaceDetails)
		}
	}
}

// NetworkInterfaceUpdateHandler handles updates to network interfaces
func NetworkInterfaceUpdateHandler(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only accept POST requests
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse form data
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form data", http.StatusBadRequest)
			return
		}

		// Get interface name from URL
		name := strings.TrimPrefix(r.URL.Path, "/api/network/interface/")
		if name == "" {
			http.Error(w, "Interface name is required", http.StatusBadRequest)
			return
		}

		// Get action from form
		action := r.Form.Get("action")

		// Handle interface state change
		if action == "up" || action == "down" {
			// Execute ip link set command
			cmd := exec.Command("ip", "link", "set", name, action)
			err := cmd.Run()
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to set interface %s %s", name, action), http.StatusInternalServerError)
				return
			}

			// If this is an HTMX request, refresh the network interfaces
			if r.Header.Get("HX-Request") == "true" {
				// Get updated interfaces
				NetworkHandler(tmpl)(w, r)
			} else {
				// Return success JSON
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{
					"status":  "success",
					"message": fmt.Sprintf("Interface %s set %s", name, action),
				})
			}
			return
		}

		// Handle IP address configuration
		ip := r.Form.Get("ip")
		mask := r.Form.Get("mask")
		if ip != "" && mask != "" {
			// Delete existing IP addresses
			cmd := exec.Command("ip", "addr", "flush", "dev", name)
			err := cmd.Run()
			if err != nil {
				http.Error(w, "Failed to flush IP addresses", http.StatusInternalServerError)
				return
			}

			// Add new IP address
			cmd = exec.Command("ip", "addr", "add", fmt.Sprintf("%s/%s", ip, mask), "dev", name)
			err = cmd.Run()
			if err != nil {
				http.Error(w, "Failed to set IP address", http.StatusInternalServerError)
				return
			}

			// Bring the interface up
			cmd = exec.Command("ip", "link", "set", name, "up")
			err = cmd.Run()
			if err != nil {
				http.Error(w, "Failed to bring interface up", http.StatusInternalServerError)
				return
			}

			// If gateway is provided, add a default route
			gateway := r.Form.Get("gateway")
			if gateway != "" {
				// Delete existing default routes for this interface
				cmd = exec.Command("ip", "route", "del", "default", "dev", name)
				_ = cmd.Run() // Ignore errors, as the route might not exist

				// Add new default route
				cmd = exec.Command("ip", "route", "add", "default", "via", gateway, "dev", name)
				err = cmd.Run()
				if err != nil {
					http.Error(w, "Failed to set default gateway", http.StatusInternalServerError)
					return
				}
			}

			// If this is an HTMX request, refresh the interface details
			if r.Header.Get("HX-Request") == "true" {
				NetworkInterfaceHandler(tmpl)(w, r)
			} else {
				// Return success JSON
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{
					"status":  "success",
					"message": fmt.Sprintf("Interface %s configuration updated", name),
				})
			}
			return
		}

		// If we got here, no valid action was taken
		http.Error(w, "No valid action specified", http.StatusBadRequest)
	}
}
