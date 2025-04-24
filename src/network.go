package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"sort"
)

type Interface struct {
	Name      string     `json:"ifname"`
	State     string     `json:"operstate,omitempty"`
	HWAddr    string     `json:"address,omitempty"`
	Addresses []AddrInfo `json:"addr_info,omitempty"`
}

type AddrInfo struct {
	Address   string `json:"local"`
	PrefixLen int    `json:"prefixlen"`
}

type Route struct {
	Destination string `json:"dst,omitempty"`
	Gateway     string `json:"gateway,omitempty"`
	Device      string `json:"dev,omitempty"`
	Protocol    string `json:"protocol,omitempty"`
	Metric      int    `json:"metric,omitempty"`
}

type NetInfo struct {
	Interfaces []Interface
	Routes4    []Route
	Routes6    []Route
}

func networkHandler(w http.ResponseWriter, r *http.Request) {
	ifaces, err := getNetworkInterfaces()
	if err != nil {
		log.Printf("Error getting network info: %v", err)
		http.Error(w, "Failed to get network interfaces", http.StatusInternalServerError)
		return
	}

	routes4, err := getNetworkRoutes(false)
	if err != nil {
		http.Error(w, "Failed to get IPv4 routes", http.StatusInternalServerError)
		return
	}

	routes6, err := getNetworkRoutes(true)
	if err != nil {
		http.Error(w, "Failed to get IPv6 routes", http.StatusInternalServerError)
		return
	}

	info := &NetInfo{
		Interfaces: ifaces,
		Routes4:    routes4,
		Routes6:    routes6,
	}

	renderPage(w, r, "net", info)
}

func getNetworkInterfaces() ([]Interface, error) {
	var interfaces []Interface
	var loopback []Interface
	var others []Interface

	cmd := exec.Command("ip", "-j", "addr")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute ip command: %w", err)
	}

	if err := json.Unmarshal(output, &interfaces); err != nil {
		return nil, fmt.Errorf("failed to parse ip command output: %w", err)
	}

	// Simple sort: loopback first, then alphabetically
	for _, iface := range interfaces {
		if iface.Name == "lo" {
			loopback = append(loopback, iface)
		} else {
			others = append(others, iface)
		}
	}

	// Sort other interfaces by name
	sort.Slice(others, func(i, j int) bool {
		return others[i].Name < others[j].Name
	})

	return append(loopback, others...), nil
}

func getNetworkRoutes(ipv6 bool) ([]Route, error) {
	var routes []Route
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

	if err := json.Unmarshal(output, &routes); err != nil {
		return nil, fmt.Errorf("failed to parse ip command output: %w", err)
	}

	for i := range routes {
		if routes[i].Gateway != "" {
			continue
		}

		if ipv6 {
			routes[i].Gateway = "::"
		} else {
			routes[i].Gateway = "0.0.0.0"
		}
	}

	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Destination == "" || routes[i].Destination == "default" {
			return true
		}
		if routes[j].Destination == "" || routes[j].Destination == "default" {
			return false
		}

		return routes[i].Destination < routes[j].Destination
	})

	return routes, nil
}
