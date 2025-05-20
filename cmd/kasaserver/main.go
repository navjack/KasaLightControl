package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"sort"
	"time"

	"github.com/user/kasalightcontrol/kasa"
)

const (
	port = 8080
)

// global server instance to allow shutdown
var httpServer *http.Server

// Device represents a Kasa device with relevant info for the UI
type Device struct {
	DeviceID   string `json:"deviceId"`
	Alias      string `json:"alias"`
	Model      string `json:"model"`
	IP         string `json:"ip"`
	MACAddress string `json:"mac_address"`      // Changed from MAC, updated tag
	IsColor    bool   `json:"is_color"`         // Updated tag
	IsDimmable bool   `json:"is_dimmable"`      // New field
	RelayState bool   `json:"relay_state"`      // Changed from IsOn, updated tag
	Status     string `json:"status,omitempty"` // Status for UI, e.g., "Online", "Discovered"
	Hue        int    `json:"hue,omitempty"`
	Saturation int    `json:"saturation,omitempty"`
	Brightness int    `json:"brightness,omitempty"`
	ColorTemp  int    `json:"color_temp,omitempty"`
}

// Store discovered devices globally for convenience
var discoveredDevices []Device

func main() {
	// Set up HTTP routes
	mux := http.NewServeMux() // Create a new ServeMux
	mux.HandleFunc("/api/discover", handleDiscover)
	mux.HandleFunc("/api/device-details", handleGetDeviceDetails)
	mux.HandleFunc("/api/set-power", handleSetPower)
	mux.HandleFunc("/api/set-light-state", handleSetLightState)
	mux.HandleFunc("/api/shutdown", handleShutdown) // New shutdown endpoint

	// Serve static files - use the mux
	mux.Handle("/", http.FileServer(http.Dir("cmd/kasaserver/static")))

	// Start the server
	serverAddr := fmt.Sprintf("localhost:%d", port)
	httpServer = &http.Server{ // Assign to the global variable
		Addr:    serverAddr,
		Handler: mux, // Use the mux
	}

	log.Printf("Starting Kasa Light Control server on http://%s", serverAddr)

	// Open browser automatically
	go openBrowser(fmt.Sprintf("http://%s", serverAddr))

	// Start the server
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Could not listen on %s: %v\n", serverAddr, err)
	}
	log.Println("Server gracefully stopped") // Message after server stops
}

// openBrowser tries to open the URL in a browser
func openBrowser(url string) {
	time.Sleep(500 * time.Millisecond) // Give server a moment to start
	var err error

	switch runtime.GOOS {
	case "darwin":
		err = exec.Command("open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default: // "linux", "freebsd", etc.
		err = exec.Command("xdg-open", url).Start()
	}

	if err != nil {
		log.Printf("Error opening browser: %v", err)
	}
}

// API handlers
func handleDiscover(w http.ResponseWriter, r *http.Request) {
	log.Println("API: Discovering devices...")

	devices, err := kasa.DiscoverDevices(5 * time.Second)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to discover devices: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Found %d devices", len(devices))

	// Convert to []Device for the API response
	discoveredDevices = make([]Device, len(devices))
	for i, d := range devices {
		discoveredDevices[i] = Device{
			IP:         d.IP,
			Alias:      d.Alias,
			Model:      d.Model,
			DeviceID:   "",    // Default value
			MACAddress: "",    // Default value
			IsColor:    false, // Default value
			IsDimmable: false, // Default value
			Status:     "Discovered", // Initial status
		}
	}

	// Sort devices by alias for consistent ordering
	sort.Slice(discoveredDevices, func(i, j int) bool {
		return discoveredDevices[i].Alias < discoveredDevices[j].Alias
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(discoveredDevices)
}

func handleGetDeviceDetails(w http.ResponseWriter, r *http.Request) {
	ip := r.URL.Query().Get("ip")
	if ip == "" {
		http.Error(w, "Missing IP parameter", http.StatusBadRequest)
		return
	}

	log.Printf("Fetching details for IP: %s", ip)
	sysInfo, err := kasa.GetSysInfo(ip)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to get device system info: " + err.Error()})
		return
	}

	// Construct the detailed device object
	detailedDevice := Device{
		IP:         ip,
		Alias:      sysInfo.Alias,
		Model:      sysInfo.Model,
		DeviceID:   sysInfo.DeviceID,
		MACAddress: sysInfo.Mac, // Default to sysInfo.Mac
		IsColor:    sysInfo.IsColor == 1,
		IsDimmable: sysInfo.IsDimmable == 1,
		Status:     "Online", // If GetSysInfo is successful, device is Online
	}

	if sysInfo.Mac == "" && sysInfo.MicMac != "" {
		detailedDevice.MACAddress = sysInfo.MicMac // Use MicMac if Mac is empty
	}

	if sysInfo.LightState != nil {
		detailedDevice.RelayState = sysInfo.LightState.OnOff == 1
		detailedDevice.Hue = sysInfo.LightState.Hue
		detailedDevice.Saturation = sysInfo.LightState.Saturation
		detailedDevice.Brightness = sysInfo.LightState.Brightness
		detailedDevice.ColorTemp = sysInfo.LightState.ColorTemp
	} else {
		// For devices without LightState (e.g., plugs that are not lights, or error fetching LightState part)
		// Check sysInfo for a general relay_state if applicable for non-light devices (not present in KasaSystemInfo currently)
		// For now, if no LightState, assume it's not a light or power state is unknown for light features.
		detailedDevice.RelayState = false // Default for safety if not a light
	}

	// Update the global list (or specific device if already present)
	// This helps keep the main list somewhat updated, though full sync is better via discover
	found := false
	for i, dev := range discoveredDevices {
		if dev.IP == ip {
			discoveredDevices[i] = detailedDevice // Update existing device
			found = true
			break
		}
	}
	if !found {
		// This case should ideally not happen if device was discovered first
		// But as a fallback, add it.
		discoveredDevices = append(discoveredDevices, detailedDevice)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detailedDevice)
}

type SetPowerRequest struct {
	IP   string `json:"ip"`
	On   bool   `json:"on"`
}

func handleSetPower(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse the request body
	var req SetPowerRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	log.Printf("API: Setting power for %s to %v", req.IP, req.On)

	// Find the device in our list to get its alias for logging
	var deviceAlias string
	for _, d := range discoveredDevices {
		if d.IP == req.IP {
			deviceAlias = d.Alias
			break
		}
	}

	// Get current system info to check if it's a bulb
	sysInfo, err := kasa.GetSysInfo(req.IP)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get device info: %v", err), http.StatusInternalServerError)
		return
	}

	// Prepare command based on device type
	simpleDesiredState := make(map[string]interface{})

	if sysInfo.LightState != nil { // KL130 bulb
		simpleDesiredState["on_off"] = ternInt(req.On, 1, 0)
	} else {
		errMsg := fmt.Sprintf("Device %s (%s) is not a recognized bulb type", deviceAlias, req.IP)
		log.Println(errMsg)
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}

	// Send command
	_, err = kasa.SetLightState(req.IP, simpleDesiredState)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to set power: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully set power for %s to %v", deviceAlias, req.On)

	// Return success response
	response := map[string]interface{}{
		"status": "success",
		"ip":     req.IP,
		"on_off": ternInt(req.On, 1, 0),
	}
	json.NewEncoder(w).Encode(response)
}

type SetLightStateRequest struct {
	IP         string `json:"ip"`
	On         bool   `json:"on"`                 // Whether the light should be on or off
	Hue        int    `json:"hue,omitempty"`        // 0-360
	Saturation int    `json:"saturation,omitempty"` // 0-100
	Brightness int    `json:"brightness,omitempty"` // 0-100
	ColorTemp  int    `json:"color_temp,omitempty"` // 0 for color mode, or Kelvin (e.g., 2700-6500)
}

func handleSetLightState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
		return
	}

	var req SetLightStateRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request payload: " + err.Error()})
		return
	}

	if req.IP == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "IP address is required"})
		return
	}

	desiredState := make(map[string]interface{})
	if req.On {
		desiredState["on_off"] = 1
	} else {
		desiredState["on_off"] = 0
	}

	// Determine mode based on request
	if req.ColorTemp > 0 { // Explicit request for white mode
		desiredState["color_temp"] = req.ColorTemp
		desiredState["hue"] = 0        // Kasa bulbs expect hue/sat to be 0 for temp mode
		desiredState["saturation"] = 0 
		if req.Brightness != 0 { // Brightness is valid in white mode
			desiredState["brightness"] = req.Brightness
		} else {
			// If brightness is 0 in the request, and bulb is being turned on,
			// Kasa might default to a previous brightness or not light up.
			// Frontend should ideally always send a valid brightness (e.g., current slider value > 0 if 'on').
			// If req.On is true and req.Brightness is 0, this might be an issue.
			// For now, we are trusting the incoming req.Brightness. If it's 0, it's sent as 0.
		}
	} else { // Color mode (hue/saturation/brightness take precedence)
		desiredState["hue"] = req.Hue
		desiredState["saturation"] = req.Saturation
		desiredState["brightness"] = req.Brightness
		desiredState["color_temp"] = 0 // Crucial for color mode
	}

	log.Printf("Setting light state for %s: %+v", req.IP, desiredState)

	lightState, err := kasa.SetLightState(req.IP, desiredState)
	if err != nil {
		log.Printf("Error setting light state for %s: %v", req.IP, err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to set light state: " + err.Error()})
		return
	}

	response := map[string]interface{}{
		"status":     "success",
		"ip":         req.IP,
		"on_off":     lightState.OnOff,
		"hue":        lightState.Hue,
		"saturation": lightState.Saturation,
		"brightness": lightState.Brightness,
		"color_temp": lightState.ColorTemp,
	}
	json.NewEncoder(w).Encode(response)
}

// handleShutdown gracefully shuts down the server
func handleShutdown(w http.ResponseWriter, r *http.Request) {
	log.Println("Shutdown request received")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Server is shutting down..."))

	// Create a context with a timeout for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		// Give a moment for the HTTP response to be sent
		time.Sleep(1 * time.Second)
		if err := httpServer.Shutdown(ctx); err != nil {
			log.Printf("Error during server shutdown: %v", err)
		}
		log.Println("Server shutdown complete. Exiting application.")
	}()
}

// Helper functions
func formatMAC(mac string) string {
	if len(mac) == 12 {
		return fmt.Sprintf("%s:%s:%s:%s:%s:%s",
			mac[0:2], mac[2:4], mac[4:6],
			mac[6:8], mac[8:10], mac[10:12])
	}
	return mac
}

func ternStr(condition bool, trueVal, falseVal string) string {
	if condition {
		return trueVal
	}
	return falseVal
}

func ternInt(condition bool, trueVal, falseVal int) int {
	if condition {
		return trueVal
	}
	return falseVal
}
