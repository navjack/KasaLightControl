package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/mux"

	"github.com/user/kasalightcontrol/kasa"
)

const (
	defaultPort = 8080
)

var (
	deviceDetailsCache      map[string]Device
	naturalLightSyncDevices map[string]bool
	lastFetchedTime         map[string]time.Time
	deviceDetailsMutex      sync.RWMutex
	naturalLightSyncMutex   sync.Mutex
	serverInstance          *http.Server // Global server instance for shutdown
	portFlag                *int         // Command-line flag for port
)

func init() {
	// Define command-line flag for the port
	portFlag = flag.Int("port", 8080, "Port for the server to listen on")
}

type Device struct {
	DeviceID     string `json:"deviceId"`
	Model        string `json:"model"`
	Alias        string `json:"alias"`
	IP           string `json:"ip"`
	MACAddress   string `json:"macAddress"`
	IsColor      bool   `json:"isColor"`
	IsDimmer     bool   `json:"isDimmer"`
	IsVariableCT bool   `json:"isVariableColorTemperature"`
	PowerState   bool   `json:"powerState"` // true for on, false for off
	Brightness   int    `json:"brightness,omitempty"`
	ColorTemp    int    `json:"colorTemp,omitempty"`
	Hue          int    `json:"hue,omitempty"`
	Saturation   int    `json:"saturation,omitempty"`
	Status       string `json:"status,omitempty"` // Added to retain discovered/online status
	IsNaturalLightActive bool `json:"isNaturalLightActive,omitempty"` // For UI to know NLS status
}

type NaturalLightPoint struct {
	Hour       int // Hour of the day (0-23)
	Minute     int // Minute of the hour (0-59)
	ColorTemp  int // Kelvin
	Brightness int // Percent (0-100)
}

var (
	discoveredDevices      []kasa.DiscoveredDevice
	discoveredDevicesMutex = &sync.Mutex{}
	naturalLightCurve      = []NaturalLightPoint{
		{Hour: 0, Minute: 0, ColorTemp: 2200, Brightness: 5},   // Midnight
		{Hour: 6, Minute: 0, ColorTemp: 2700, Brightness: 10},  // Sunrise start
		{Hour: 7, Minute: 30, ColorTemp: 3500, Brightness: 40}, // Morning
		{Hour: 9, Minute: 0, ColorTemp: 4500, Brightness: 70},  // Late Morning
		{Hour: 12, Minute: 0, ColorTemp: 6000, Brightness: 100}, // Solar Noon
		{Hour: 15, Minute: 0, ColorTemp: 5000, Brightness: 80}, // Afternoon
		{Hour: 17, Minute: 0, ColorTemp: 4000, Brightness: 60}, // Late Afternoon
		{Hour: 18, Minute: 30, ColorTemp: 3000, Brightness: 40}, // Sunset
		{Hour: 21, Minute: 0, ColorTemp: 2500, Brightness: 15}, // Evening
		{Hour: 22, Minute: 30, ColorTemp: 2200, Brightness: 5},  // Late Evening
	}
	naturalLightUpdateInterval = 5 * time.Minute // How often to update
	naturalLightTransitionPeriod = 5000        // 5 seconds in milliseconds
	naturalLightTicker      *time.Ticker
	naturalLightStopChan    chan struct{}
)

func calculateNaturalLightState(t time.Time) (colorTemp int, brightness int) {
	// Ensure naturalLightCurve is sorted by time (should be done at startup once)
	// For safety, could re-sort or check here, but assume sorted for performance.

	nowMinutes := t.Hour()*60 + t.Minute()
	var p1, p2 NaturalLightPoint
	var p1TimeInMinutes, p2TimeInMinutes int

	// Find points p1 (before/at now) and p2 (after now)
	found := false
	for i := 0; i < len(naturalLightCurve); i++ {
		p1 = naturalLightCurve[i]
		p1TimeInMinutes = p1.Hour*60 + p1.Minute

		p2 = naturalLightCurve[(i+1)%len(naturalLightCurve)] // Wrap around for the last point
		p2TimeInMinutes = p2.Hour*60 + p2.Minute

		effectiveP2Minutes := p2TimeInMinutes
		if effectiveP2Minutes < p1TimeInMinutes { // p2 is on the next day (e.g., p1 is 22:00, p2 is 00:00)
			effectiveP2Minutes += 24 * 60
		}

		if nowMinutes >= p1TimeInMinutes && nowMinutes < effectiveP2Minutes {
			found = true
			break
		}
		// Handle the case where current time is exactly the last point, or between last point and midnight
		// and the next point is the first point of the day (wrap-around).
		if i == len(naturalLightCurve)-1 && nowMinutes >= p1TimeInMinutes { // Current time is at or after the last defined point
			found = true // p1 is the last point, p2 is the first point (wrapped)
			break
		}
	}

	if !found {
		// Should not happen if curve is well-defined and covers 24h, or if logic is perfect.
		// Default to the first point in the curve if something goes wrong.
		log.Printf("Warning: Could not accurately find bracketing points for time %v. Defaulting to first curve point.", t)
		return naturalLightCurve[0].ColorTemp, naturalLightCurve[0].Brightness
	}

	// Interpolate
	effectiveP2TimeInMinutes := p2TimeInMinutes
	if effectiveP2TimeInMinutes < p1TimeInMinutes { // p2 is on the next day
		effectiveP2TimeInMinutes += 24 * 60
	}

	denominator := float64(effectiveP2TimeInMinutes - p1TimeInMinutes)
	if denominator == 0 { // p1 and p2 are effectively the same time point
		return p1.ColorTemp, p1.Brightness
	}

	// Adjust nowMinutes if it's part of the wrapped period (e.g. p1=22:00, p2=01:00(next day), now=23:00)
	effectiveNowMinutes := float64(nowMinutes)
	// If p1 is late in day, and p2 is early next day, and nowMinutes is *also* early next day but numerically smaller than p1's minutes
	if p1TimeInMinutes > p2TimeInMinutes && nowMinutes < p1TimeInMinutes && nowMinutes < p2TimeInMinutes {
		// This condition means nowMinutes is something like 00:30, p1 is 22:00, p2 is 06:00.
		// This case is handled by the loop finding p1=naturalLightCurve[0] and p2=naturalLightCurve[1]
	}

	// The crucial part for interpolation factor:
	// If we wrapped (p1 is late, p2 is early next day), and 'now' is also late (after p1), then 'now' is correct.
	// If we wrapped, and 'now' is early next day (numerically smaller than p1), then effectiveNowMinutes needs to be now + 24*60
	if p1TimeInMinutes > p2TimeInMinutes && nowMinutes < p1TimeInMinutes { // This implies 'now' is on the next day relative to p1
		effectiveNowMinutes += 24 * 60
	}

	tFactor := (effectiveNowMinutes - float64(p1TimeInMinutes)) / denominator

	if tFactor < 0 { tFactor = 0 }
	if tFactor > 1 { tFactor = 1 }

	ct := int(float64(p1.ColorTemp) + tFactor*(float64(p2.ColorTemp-p1.ColorTemp)))
	br := int(float64(p1.Brightness) + tFactor*(float64(p2.Brightness-p1.Brightness)))

	return ct, br
}

func startNaturalLightSyncManager() {
	// Ensure naturalLightCurve is sorted by time at startup
	sort.Slice(naturalLightCurve, func(i, j int) bool {
		timeI := naturalLightCurve[i].Hour*60 + naturalLightCurve[i].Minute
		timeJ := naturalLightCurve[j].Hour*60 + naturalLightCurve[j].Minute
		return timeI < timeJ
	})

	naturalLightTicker = time.NewTicker(naturalLightUpdateInterval)
	naturalLightStopChan = make(chan struct{})
	log.Println("Natural Light Sync manager started. Update interval:", naturalLightUpdateInterval)

	go func() {
		for {
			select {
			case <-naturalLightTicker.C:
				updateAllNaturalLightDevices()
			case <-naturalLightStopChan:
				naturalLightTicker.Stop()
				log.Println("Natural Light Sync manager stopped.")
				return
			}
		}
	}()
}

func stopNaturalLightSyncManager() {
	if naturalLightStopChan != nil {
		// Check if channel is already closed to prevent panic
		select {
		case _, ok := <-naturalLightStopChan:
			if ok { // Channel is open and received something (should not happen)
				close(naturalLightStopChan)
			} // if !ok, channel is already closed.
		default: // Channel is open and would block, so close it.
			close(naturalLightStopChan)
		}
	}
}

func updateAllNaturalLightDevices() {
	naturalLightSyncMutex.Lock()
	activeDevices := make([]string, 0, len(naturalLightSyncDevices))
	for ip, enabled := range naturalLightSyncDevices {
		if enabled {
			activeDevices = append(activeDevices, ip)
		}
	}
	naturalLightSyncMutex.Unlock()

	if len(activeDevices) == 0 {
		return
	}
	log.Printf("Natural Light Sync: Updating %d devices...", len(activeDevices))

	currentTime := time.Now()
	colorTemp, brightness := calculateNaturalLightState(currentTime)
	log.Printf("Natural Light Sync: Calculated State for %v: Temp=%dK, Brightness=%d%%", currentTime.Format(time.Kitchen), colorTemp, brightness)

	onOff := 1
	if brightness <= 0 {
		onOff = 0 // If calculated brightness is 0, turn off.
		// Kasa API might require specific handling for 'off' via brightness 0.
		// For safety, let's ensure brightness is at least 1 if on_off is 1, but Kasa might handle it.
		// The current curve keeps brightness > 0.
	}

	desiredState := map[string]interface{}{
		"on_off":            onOff,
		"color_temp":        colorTemp,
		"brightness":        brightness,
		"hue":               0, // Explicitly 0 for white mode
		"saturation":        0, // Explicitly 0 for white mode
		"transition_period": naturalLightTransitionPeriod,
	}

	for _, ip := range activeDevices {
		log.Printf("Natural Light Sync: Setting state for %s: %+v", ip, desiredState)
		_, err := kasa.SetLightState(ip, desiredState)
		if err != nil {
			log.Printf("Natural Light Sync: Error setting state for %s: %v", ip, err)
			// Optional: Disable sync for this device after multiple errors.
		}
	}
}

func handleSetNaturalLightMode(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	ip := vars["ip"]
	if ip == "" {
		http.Error(w, "IP address is required", http.StatusBadRequest)
		return
	}

	var reqBody struct {
		Enable bool `json:"enable"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	naturalLightSyncMutex.Lock()
	previouslyEnabled := naturalLightSyncDevices[ip]
	if reqBody.Enable {
		naturalLightSyncDevices[ip] = true
		log.Printf("Natural Light Sync enabled for %s", ip)
	} else {
		delete(naturalLightSyncDevices, ip)
		log.Printf("Natural Light Sync disabled for %s", ip)
	}
	naturalLightSyncMutex.Unlock()

	// Trigger an immediate update if enabling for the first time or re-enabling
	if reqBody.Enable && !previouslyEnabled {
		log.Printf("Natural Light Sync: Triggering immediate update for %s", ip)
		go func(devIP string) {
			currentTime := time.Now()
			colorTemp, brightness := calculateNaturalLightState(currentTime)
			onOff := 1
			if brightness <= 0 { onOff = 0 }
			desiredState := map[string]interface{}{
				"on_off":            onOff,
				"color_temp":        colorTemp,
				"brightness":        brightness,
				"hue":               0,
				"saturation":        0,
				"transition_period": naturalLightTransitionPeriod, // Use transition for initial set too
			}
			log.Printf("Natural Light Sync: Initial state for %s: %+v", devIP, desiredState)
			_, err := kasa.SetLightState(devIP, desiredState)
			if err != nil {
				log.Printf("Natural Light Sync: Error setting initial state for %s: %v", devIP, err)
			}
		}(ip)
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Natural Light Sync for %s set to %v", ip, reqBody.Enable)
}

func setupRoutes() *mux.Router {
	r := mux.NewRouter()

	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/discover", handleDiscover).Methods("GET")
	api.HandleFunc("/devices", handleGetDevices).Methods("GET")
	api.HandleFunc("/device/{ip}/details", getDeviceDetailsHandler).Methods("GET")
	api.HandleFunc("/set-light-state", handleSetLightState).Methods("POST")
	api.HandleFunc("/device/{ip}/light-state", handleSetLightStateWithIP).Methods("POST")
	api.HandleFunc("/device/{ip}/natural-light", handleSetNaturalLightMode).Methods("POST")
	api.HandleFunc("/set-power", handleSetPower).Methods("POST")     // Ensure this is also using Gorilla Mux vars if needed
	api.HandleFunc("/shutdown", handleShutdown).Methods("POST") // New shutdown endpoint

	// Serve static files
	staticDir := "./cmd/kasaserver/static" // Corrected path
	fileServer := http.FileServer(http.Dir(staticDir))
	r.PathPrefix("/").Handler(http.StripPrefix("/", fileServer))

	return r
}

func startServer(port int) {
	r := setupRoutes()

	serverInstance = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: r,
	}

	log.Printf("Server listening on port %d", port)
	log.Printf("Web UI available at http://localhost:%d", port)

	go func() {
		if err := serverInstance.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe(): %v", err)
		}
	}()
	openBrowser(fmt.Sprintf("http://localhost:%d", port))
}

func main() {
	flag.Parse() // Parse command-line flags

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Kasa Light Control Server starting...")

	// Initialize caches and maps
	deviceDetailsCache = make(map[string]Device)
	lastFetchedTime = make(map[string]time.Time)
	naturalLightSyncDevices = make(map[string]bool)

	startNaturalLightSyncManager() // Start the natural light sync manager

	startServer(*portFlag) // Start the HTTP server with the configured port

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	stopNaturalLightSyncManager() // Stop the natural light sync manager

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if serverInstance != nil {
		if err := serverInstance.Shutdown(ctx); err != nil {
			log.Fatalf("Server Shutdown Failed:%+v", err)
		}
	}
	log.Println("Server shutdown complete. Exiting application.")
}

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

func handleDiscover(w http.ResponseWriter, r *http.Request) {
	log.Println("API: Discovering devices...")

	discoveredDevices, err := kasa.DiscoverDevices(5 * time.Second)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to discover devices: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Found %d devices", len(discoveredDevices))

	// Convert to []Device for the API response
	uiDevices := make([]Device, len(discoveredDevices))
	for i, kasaDev := range discoveredDevices {
		// Try to get details for already known device if full scan wasn't requested
		deviceDetailsMutex.Lock()
		d, exists := deviceDetailsCache[kasaDev.IP]
		deviceDetailsMutex.Unlock()
		if exists {
			// If basic details exist, just use them. UI can request full details later.
			uiDevices[i] = d // d is already Device type
			continue
		}

		// If not in cache, create a basic entry
		uiDev := Device{
			IP:         kasaDev.IP,
			Model:      kasaDev.Model,
			Alias:      kasaDev.Alias,
			// MACAddress: kasaDev.MAC, // MAC is not in DiscoveredDevice, will be fetched by GetSysInfo
			Status:     "Discovered",
		}
		uiDevices[i] = uiDev
		deviceDetailsMutex.Lock()
		deviceDetailsCache[kasaDev.IP] = uiDev
		deviceDetailsMutex.Unlock()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(uiDevices)
}

func getDeviceDetailsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	ip := vars["ip"]
	if ip == "" {
		http.Error(w, "IP address is required", http.StatusBadRequest)
		return
	}

	log.Printf("Fetching details for IP: %s", ip)

	var foundInDiscovery bool
	discoveredDevicesMutex.Lock()
	for _, dev := range discoveredDevices {
		if dev.IP == ip {
			foundInDiscovery = true
			break
		}
	}
	discoveredDevicesMutex.Unlock()

	// Always fetch fresh SysInfo to get the most up-to-date details and state.
	sysInfo, err := kasa.GetSysInfo(ip)
	if err != nil {
		// If not found in discovery and GetSysInfo fails, then it's truly unreachable or an error.
		if !foundInDiscovery {
			http.Error(w, "Failed to get device info (not in discovery and GetSysInfo failed): "+err.Error(), http.StatusInternalServerError)
			return
		}
		// If it was in discovery but GetSysInfo failed now, log it but we might proceed with cached basic info if desired.
		// For now, we'll treat it as an error for fetching details.
		log.Printf("Error fetching SysInfo for %s (was in discovery): %v. Cache will not be updated with full details.", ip, err)
		// Optionally, one could return the cached basic details if they exist and are deemed acceptable.
		// For now, we demand fresh SysInfo for this endpoint.
		http.Error(w, "Failed to get fresh device system info: "+err.Error(), http.StatusInternalServerError)
		return
	}

	detailedDevice := Device{
		IP:           ip,
		DeviceID:     sysInfo.DeviceID,
		Model:        sysInfo.Model,
		Alias:        sysInfo.Alias,
		MACAddress:   sysInfo.Mac, 
		IsColor:      sysInfo.IsColor == 1,
		IsDimmer:     sysInfo.IsDimmable == 1,
		IsVariableCT: sysInfo.IsVariableColorTemp == 1, // Corrected field name
		PowerState:   false, // Default, will be updated by LightState or RelayState
		Status:       "Online",
	}
	// Use MicMac if Mac is empty, as MicMac is usually the reliable physical MAC
	if detailedDevice.MACAddress == "" && sysInfo.MicMac != "" {
	    detailedDevice.MACAddress = sysInfo.MicMac
	}

	if sysInfo.LightState != nil {
		detailedDevice.PowerState = sysInfo.LightState.OnOff == 1
		detailedDevice.Brightness = sysInfo.LightState.Brightness
		detailedDevice.ColorTemp = sysInfo.LightState.ColorTemp
		detailedDevice.Hue = sysInfo.LightState.Hue
		detailedDevice.Saturation = sysInfo.LightState.Saturation
	} else {
		// For devices without LightState (e.g., plugs that are not lights)
		// Check sysInfo for a general relay_state if applicable (e.g. sysInfo.RelayState for plugs)
		// For now, assume if no LightState, it might be a non-light device or error.
		// If it's a smart plug, sysInfo.RelayState (0 or 1) is the field.
		if devType, _ := kasa.GetDeviceTypeFromModel(sysInfo.Model); devType == kasa.SmartPlug {
			detailedDevice.PowerState = sysInfo.RelayState == 1 // Assuming RelayState exists in KasaSystemInfo for plugs
		} else {
			detailedDevice.PowerState = false // Default for safety if not a light or plug with known state
		}
	}

	deviceDetailsMutex.Lock()
	deviceDetailsCache[ip] = detailedDevice
	deviceDetailsMutex.Unlock()

	// Check NLS status
	naturalLightSyncMutex.Lock()
	_, detailedDevice.IsNaturalLightActive = naturalLightSyncDevices[ip]
	naturalLightSyncMutex.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detailedDevice)
}

func handleGetDevices(w http.ResponseWriter, r *http.Request) {
	deviceDetailsMutex.Lock()
	devices := make([]Device, 0, len(deviceDetailsCache))
	for _, dev := range deviceDetailsCache {
		devices = append(devices, dev)
	}
	deviceDetailsMutex.Unlock()

	// Optionally sort the devices, e.g., by IP or Alias
	sort.Slice(devices, func(i, j int) bool {
		return devices[i].IP < devices[j].IP // Sort by IP for consistency
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(devices)
}

func handleSetPower(w http.ResponseWriter, r *http.Request) {
	var reqBody struct {
		IP   string `json:"ip"`
		IsOn bool   `json:"isOn"` // Changed field name and added tag to match JS
	}

	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		http.Error(w, "Invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("API: handleSetPower invoked for IP: %s, IsOn: %t", reqBody.IP, reqBody.IsOn)

	var err error
	if reqBody.IsOn {
		_, err = kasa.TurnOn(reqBody.IP)
	} else {
		_, err = kasa.TurnOff(reqBody.IP)
	}

	if err != nil {
		http.Error(w, "Failed to set power state: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Update cache
	deviceDetailsMutex.Lock()
	if dev, ok := deviceDetailsCache[reqBody.IP]; ok {
		dev.PowerState = reqBody.IsOn
		deviceDetailsCache[reqBody.IP] = dev
	}
	deviceDetailsMutex.Unlock()

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Power state set for %s", reqBody.IP)
}

func handleSetLightState(w http.ResponseWriter, r *http.Request) {
	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	ip, ok := payload["ip"].(string)
	if !ok || ip == "" {
		http.Error(w, "IP address is required and must be a string", http.StatusBadRequest)
		return
	}

	// Construct the desiredLightState map for kasa.SetLightState
	desiredLightState := make(map[string]interface{})
	copyableFields := []string{"on_off", "hue", "saturation", "brightness", "color_temp", "transition_period"}

	// Explicitly set on_off to 1 if any light-changing parameter is present and on_off is not set to 0
	setOn := false
	for key, value := range payload {
		if key != "ip" && key != "on_off" && value != nil {
			switch v := value.(type) {
			case float64:
				if v != 0 { setOn = true }
			case int:
				if v != 0 { setOn = true }
			}
			if setOn { break }
		}
	}

	// If on_off is explicitly set to 0, respect that. Otherwise, if other params imply 'on', set on_off=1.
	if onOffPayload, onOffExists := payload["on_off"]; onOffExists {
		if onOffFloat, isFloat := onOffPayload.(float64); isFloat && onOffFloat == 0 {
			desiredLightState["on_off"] = 0
			setOn = false // Explicitly off
		} else if onOffInt, isInt := onOffPayload.(int); isInt && onOffInt == 0 {
			desiredLightState["on_off"] = 0
			setOn = false // Explicitly off
		}
	}

	if setOn {
		if _, exists := desiredLightState["on_off"]; !exists {
			desiredLightState["on_off"] = 1 // Default to on if other parameters are being set and not explicitly turning off
		}
	}

	for _, field := range copyableFields {
		if val, ok := payload[field]; ok && val != nil {
			// Kasa API expects integers for these values.
			// JSON unmarshals numbers into float64 by default.
			if fVal, isFloat := val.(float64); isFloat {
				desiredLightState[field] = int(fVal)
			} else {
				desiredLightState[field] = val // Assume it's already an int or other compatible type
			}
		}
	}

	// Ensure mutually exclusive HSB vs ColorTemp settings
	if hue, hueOk := desiredLightState["hue"]; hueOk && hue.(int) > 0 {
		desiredLightState["color_temp"] = 0
	} else if ct, ctOk := desiredLightState["color_temp"]; ctOk && ct.(int) > 0 {
		desiredLightState["hue"] = 0
		desiredLightState["saturation"] = 0
	}

	log.Printf("Setting light state for %s: %+v", ip, desiredLightState)

	_, err := kasa.SetLightState(ip, desiredLightState)
	if err != nil {
		http.Error(w, "Failed to set light state: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Update cache with new state (best effort, actual state might differ slightly or due to transition)
	deviceDetailsMutex.Lock()
	if dev, ok := deviceDetailsCache[ip]; ok {
		if onOff, ok := desiredLightState["on_off"]; ok {
			switch v := onOff.(type) {
			case int:
				dev.PowerState = v == 1
			case bool:
				dev.PowerState = v
			case float64:
				dev.PowerState = int(v) == 1
			}
		}
		if brightness, ok := desiredLightState["brightness"]; ok {
			if brightnessInt, ok := brightness.(int); ok {
				dev.Brightness = brightnessInt
			}
		}
		if colorTemp, ok := desiredLightState["color_temp"]; ok {
			if colorTempInt, ok := colorTemp.(int); ok {
				dev.ColorTemp = colorTempInt
			}
		}
		if hue, ok := desiredLightState["hue"]; ok {
			if hueInt, ok := hue.(int); ok {
				dev.Hue = hueInt
			}
		}
		if saturation, ok := desiredLightState["saturation"]; ok {
			if saturationInt, ok := saturation.(int); ok {
				dev.Saturation = saturationInt
			}
		}
		deviceDetailsCache[ip] = dev
	}
	deviceDetailsMutex.Unlock()

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Light state set for %s", ip)
}

// handleSetLightStateWithIP handles setting light state with IP from URL path
func handleSetLightStateWithIP(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	ip := vars["ip"]
	if ip == "" {
		http.Error(w, "IP address is required in URL path", http.StatusBadRequest)
		return
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Add the IP to the payload for the existing handleSetLightState logic
	payload["ip"] = ip

	// Construct the desiredLightState map for kasa.SetLightState
	desiredLightState := make(map[string]interface{})
	copyableFields := []string{"on_off", "hue", "saturation", "brightness", "color_temp", "transition_period"}

	// Explicitly set on_off to 1 if any light-changing parameter is present and on_off is not set to 0
	setOn := false
	for key, value := range payload {
		if key != "ip" && key != "on_off" && value != nil {
			switch v := value.(type) {
			case float64:
				if v != 0 { setOn = true }
			case int:
				if v != 0 { setOn = true }
			}
			if setOn { break }
		}
	}

	// If on_off is explicitly set to 0, respect that. Otherwise, if other params imply 'on', set on_off=1.
	if onOffPayload, onOffExists := payload["on_off"]; onOffExists {
		if onOffFloat, isFloat := onOffPayload.(float64); isFloat && onOffFloat == 0 {
			desiredLightState["on_off"] = 0
			setOn = false // Explicitly off
		} else if onOffInt, isInt := onOffPayload.(int); isInt && onOffInt == 0 {
			desiredLightState["on_off"] = 0
			setOn = false // Explicitly off
		}
	}

	if setOn {
		if _, exists := desiredLightState["on_off"]; !exists {
			desiredLightState["on_off"] = 1 // Default to on if other parameters are being set and not explicitly turning off
		}
	}

	for _, field := range copyableFields {
		if val, ok := payload[field]; ok && val != nil {
			// Kasa API expects integers for these values.
			// JSON unmarshals numbers into float64 by default.
			if fVal, isFloat := val.(float64); isFloat {
				desiredLightState[field] = int(fVal)
			} else {
				desiredLightState[field] = val // Assume it's already an int or other compatible type
			}
		}
	}

	// Ensure mutually exclusive HSB vs ColorTemp settings
	if hue, hueOk := desiredLightState["hue"]; hueOk && hue.(int) > 0 {
		desiredLightState["color_temp"] = 0
	} else if ct, ctOk := desiredLightState["color_temp"]; ctOk && ct.(int) > 0 {
		desiredLightState["hue"] = 0
		desiredLightState["saturation"] = 0
	}

	log.Printf("Setting light state for %s: %+v", ip, desiredLightState)

	_, err := kasa.SetLightState(ip, desiredLightState)
	if err != nil {
		http.Error(w, "Failed to set light state: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Update cache with new state (best effort, actual state might differ slightly or due to transition)
	deviceDetailsMutex.Lock()
	if dev, ok := deviceDetailsCache[ip]; ok {
		if onOff, ok := desiredLightState["on_off"]; ok {
			switch v := onOff.(type) {
			case int:
				dev.PowerState = v == 1
			case bool:
				dev.PowerState = v
			case float64:
				dev.PowerState = int(v) == 1
			}
		}
		if brightness, ok := desiredLightState["brightness"]; ok {
			if brightnessInt, ok := brightness.(int); ok {
				dev.Brightness = brightnessInt
			}
		}
		if colorTemp, ok := desiredLightState["color_temp"]; ok {
			if colorTempInt, ok := colorTemp.(int); ok {
				dev.ColorTemp = colorTempInt
			}
		}
		if hue, ok := desiredLightState["hue"]; ok {
			if hueInt, ok := hue.(int); ok {
				dev.Hue = hueInt
			}
		}
		if saturation, ok := desiredLightState["saturation"]; ok {
			if saturationInt, ok := saturation.(int); ok {
				dev.Saturation = saturationInt
			}
		}
		deviceDetailsCache[ip] = dev
	}
	deviceDetailsMutex.Unlock()

	// Return JSON response for consistency with frontend expectations
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Light state set for %s", ip),
	})
}

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
		if err := serverInstance.Shutdown(ctx); err != nil {
			log.Printf("Error during server shutdown: %v", err)
		}
		log.Println("Server shutdown complete. Exiting application.")
	}()
}
