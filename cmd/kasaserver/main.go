package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/user/kasalightcontrol/kasa" // Assuming this is the correct path to your kasa package
)

const serverPort = ":8080"

// writeJSONResponse is a helper to send JSON responses
func writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if data != nil {
		err := json.NewEncoder(w).Encode(data)
		if err != nil {
			log.Printf("Error encoding JSON response: %v", err)
			// Avoid writing further if headers already sent
		}
	}
}

// httpError is a helper to send JSON error responses
func httpError(w http.ResponseWriter, message string, statusCode int) {
	log.Println("HTTP Error:", message)
	writeJSONResponse(w, statusCode, map[string]string{"error": message})
}

func discoverHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpError(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	// Set a default timeout for discovery, e.g., 5 seconds
	discoveryTimeout := 5 * time.Second 
	devices, err := kasa.DiscoverDevices(discoveryTimeout) // Pass the timeout
	if err != nil {
		httpError(w, fmt.Sprintf("Failed to discover devices: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSONResponse(w, http.StatusOK, devices)
}

func sysinfoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpError(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	ip := strings.TrimPrefix(r.URL.Path, "/device/")
	ip = strings.TrimSuffix(ip, "/sysinfo")
	if ip == "" {
		httpError(w, "Device IP not specified in path", http.StatusBadRequest)
		return
	}

	sysInfo, err := kasa.GetSysInfo(ip)
	if err != nil {
		// Check if Kasa returned a specific error code (e.g., device offline)
		if sysInfo != nil && sysInfo.ErrCode != 0 {
			httpError(w, fmt.Sprintf("Kasa device error (code %d): %v", sysInfo.ErrCode, err), http.StatusNotFound) // Or map to other status
		} else {
			httpError(w, fmt.Sprintf("Failed to get sysinfo for %s: %v", ip, err), http.StatusInternalServerError)
		}
		return
	}
	writeJSONResponse(w, http.StatusOK, sysInfo)
}

func lightStateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	ip := strings.TrimPrefix(r.URL.Path, "/device/")
	ip = strings.TrimSuffix(ip, "/lightstate")
	if ip == "" {
		httpError(w, "Device IP not specified in path", http.StatusBadRequest)
		return
	}

	var desiredState map[string]interface{}
	err := json.NewDecoder(r.Body).Decode(&desiredState)
	if err != nil {
		httpError(w, fmt.Sprintf("Invalid JSON payload: %v", err), http.StatusBadRequest)
		return
	}

	returnedState, err := kasa.SetLightState(ip, desiredState)
	if err != nil {
		if returnedState != nil && returnedState.ErrCode != 0 {
			httpError(w, fmt.Sprintf("Kasa device error setting light state (code %d): %v", returnedState.ErrCode, err), http.StatusConflict) // Or other appropriate status
		} else {
			httpError(w, fmt.Sprintf("Failed to set light state for %s: %v", ip, err), http.StatusInternalServerError)
		}
		return
	}
	writeJSONResponse(w, http.StatusOK, returnedState)
}

func main() {
	http.HandleFunc("/discover", discoverHandler)
	// Note: For path parameters like IP, a simple router or more specific path matching might be better in a larger app.
	// For now, we'll use specific handlers and string manipulation for /device/:ip/ routes.
	http.HandleFunc("/device/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sysinfo") {
			sysinfoHandler(w, r)
		} else if strings.HasSuffix(r.URL.Path, "/lightstate") {
			lightStateHandler(w, r)
		} else {
			httpError(w, "Endpoint not found", http.StatusNotFound)
		}
	})

	log.Printf("Kasa Local API Server starting on port %s\n", serverPort)
	if err := http.ListenAndServe(serverPort, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
