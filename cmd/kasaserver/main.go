package main

import (
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

// Device represents a Kasa device with relevant info for the UI
type Device struct {
	DeviceID   string `json:"deviceId"`
	Alias      string `json:"alias"`
	Model      string `json:"model"`
	IsColor    bool   `json:"isColor"`
	IP         string `json:"ip"`
	MAC        string `json:"mac"`
	Status     string `json:"status"` // e.g., "Online", "Offline", "Discovered"
	IsOn       bool   `json:"isOn"`
	Hue        int    `json:"hue,omitempty"`        // 0-360, from light_state
	Saturation int    `json:"saturation,omitempty"` // 0-100, from light_state
	Brightness int    `json:"brightness,omitempty"` // 0-100, from light_state
	ColorTemp  int    `json:"color_temp,omitempty"` // Kelvin, from light_state
}

// Store discovered devices globally for convenience
var discoveredDevices []Device

func main() {
	// Set up HTTP routes
	http.HandleFunc("/api/discover", handleDiscover)
	http.HandleFunc("/api/device-details", handleGetDeviceDetails)
	http.HandleFunc("/api/set-power", handleSetPower)
	http.HandleFunc("/api/set-light-state", handleSetLightState)
	
	// Serve static files
	http.Handle("/", http.FileServer(http.Dir("cmd/kasaserver/static")))
	
	// Start the server
	serverAddr := fmt.Sprintf("localhost:%d", port)
	log.Printf("Starting Kasa Light Control server on http://%s", serverAddr)
	
	// Open browser automatically
	go openBrowser(fmt.Sprintf("http://%s", serverAddr))
	
	// Start the server
	if err := http.ListenAndServe(serverAddr, nil); err != nil {
		log.Fatal(err)
	}
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
			IP:       d.IP,
			Alias:    d.Alias,
			Model:    d.Model,
			DeviceID: "",    // Default value
			IsColor:  false, // Default value
			Status:   "Discovered", // Initial status
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
		DeviceID: sysInfo.DeviceID,
		Alias:    sysInfo.Alias,
		Model:    sysInfo.Model,
		IsColor:  sysInfo.IsColor == 1,
		IP:       ip,
		MAC:      sysInfo.Mac, // Assuming Mac is populated in GetSysInfo correctly
		Status:   "Online",    // If we got sysInfo, it's online
	}

	if sysInfo.LightState != nil {
		detailedDevice.IsOn = sysInfo.LightState.OnOff == 1
		// Only populate color/brightness details if it's a color bulb and they are meaningful
		if detailedDevice.IsColor {
			detailedDevice.Hue = sysInfo.LightState.Hue
			detailedDevice.Saturation = sysInfo.LightState.Saturation
			detailedDevice.Brightness = sysInfo.LightState.Brightness
		}
		// ColorTemp can be present for both color and non-color (tunable white) bulbs
		detailedDevice.ColorTemp = sysInfo.LightState.ColorTemp
	} else {
		// For devices without light_state (e.g., plugs), set defaults or specific logic
		// Check if it's a plug based on model or mic_type if necessary.
		// For now, we assume if no light_state, it might be a plug or non-lighting device.
		// The KasaSystemInfo struct has RelayState for plugs, but GetSysInfo currently doesn't parse it out simply.
		// To check plug state, a different command `{"system":{"get_sysinfo":null}}` would need parsing for `relay_state`
		// For simplicity, we'll rely on LightState for bulbs.
		// If it's a smart plug, IsOn might be determined by relay_state, not covered here yet.
		detailedDevice.IsOn = false // Default for non-light_state devices or if state is unknown
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

	// Only include color/brightness parameters if they are likely intended for color mode
	// If color_temp is explicitly set to a non-zero value, it implies white mode.
	// If hue/saturation/brightness are set, we assume color mode.
	if req.Hue != 0 || req.Saturation != 0 || req.Brightness != 0 {
		desiredState["hue"] = req.Hue
		desiredState["saturation"] = req.Saturation
		desiredState["brightness"] = req.Brightness
		desiredState["color_temp"] = 0 // Crucial for color mode
	} else if req.ColorTemp > 0 { // If only color_temp is specified (for white mode)
		desiredState["color_temp"] = req.ColorTemp
		// Brightness can also be set in white mode
		if req.Brightness != 0 {
			desiredState["brightness"] = req.Brightness
		}
		// Remove color params if we are setting a specific white temperature
		delete(desiredState, "hue")
		delete(desiredState, "saturation")
	}

	// If only on_off is changing, and no color/brightness/temp details are provided,
	// it will just toggle power. If other params are present, they take precedence.

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
