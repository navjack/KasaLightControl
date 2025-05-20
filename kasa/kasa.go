package kasa

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"
)

const (
	DefaultPort         = 9999
	InitializationVector = 171
	DefaultTimeout      = 5 * time.Second
	DiscoveryTimeout    = 3 * time.Second
)

// DiscoveredDevice holds information about a Kasa device found via discovery.
type DiscoveredDevice struct {
	IP    string `json:"ip"`
	Alias string `json:"alias"`
	Model string `json:"model"`
	// We can add more fields from sysinfo if needed, e.g., deviceId, MAC
}

// KasaLightState represents the light state object from Kasa responses.
type KasaLightState struct {
	OnOff      int    `json:"on_off"`
	Mode       string `json:"mode,omitempty"`
	Hue        int    `json:"hue,omitempty"`
	Saturation int    `json:"saturation,omitempty"`
	Brightness int    `json:"brightness,omitempty"`
	ColorTemp  int    `json:"color_temp,omitempty"`
	ErrCode    int    `json:"err_code,omitempty"` // Present in transition_light_state, not usually in light_state sub-object of get_sysinfo
}

// KasaSystemInfo represents the detailed system information from a Kasa device.
type KasaSystemInfo struct {
	SwVer      string          `json:"sw_ver"`
	HwVer      string          `json:"hw_ver"`
	Model      string          `json:"model"`
	DeviceID   string          `json:"deviceId"`
	OemID      string          `json:"oemId"`
	HwID       string          `json:"hwId"`
	RSSI       int             `json:"rssi"`
	LatitudeI  int             `json:"latitude_i,omitempty"`  // omitempty as it might not always be set or relevant
	LongitudeI int             `json:"longitude_i,omitempty"` // omitempty as it might not always be set or relevant
	Alias      string          `json:"alias"`
	MicType    string          `json:"mic_type"` // e.g., "IOT.SMARTBULB"
	Feature    string          `json:"feature,omitempty"` // e.g., "TIM:ENE"
	Mac        string          `json:"mac,omitempty"`     // Sometimes under mic_mac, sometimes just mac
	MicMac     string          `json:"mic_mac,omitempty"` // Physical MAC address
	IsDimmable int             `json:"is_dimmable,omitempty"`
	IsColor    int             `json:"is_color,omitempty"`
	IsVariableColorTemp int    `json:"is_variable_color_temp,omitempty"`
	LightState *KasaLightState `json:"light_state,omitempty"` // Pointer, as not all devices have light_state (e.g., plugs)
	RelayState int             `json:"relay_state,omitempty"` // For smart plugs, 0 = off, 1 = on
	ErrCode    int             `json:"err_code"`
	// We can add more fields like preferred_state, ctrl_protocols, etc. if needed
}

// GetSysinfoResponseWrapper is a helper to unmarshal the full get_sysinfo response.
type GetSysinfoResponseWrapper struct {
	System struct {
		GetSysinfo KasaSystemInfo `json:"get_sysinfo"`
	} `json:"system"`
}

// TransitionLightStateResponseWrapper is a helper to unmarshal the full transition_light_state response.
type TransitionLightStateResponseWrapper struct {
	SmartlifeIoTSmartbulbLightingservice struct {
		TransitionLightState KasaLightState `json:"transition_light_state"`
	} `json:"smartlife.iot.smartbulb.lightingservice"`
}

// encrypt performs XOR encryption on the plaintext string.
// The Kasa protocol prepends the 4-byte big-endian length of the
// original plaintext *before* encryption, but this function only returns the encrypted payload.
// The length prefixing is handled by the SendCommand function.
func encrypt(plaintext string) []byte {
	key := byte(InitializationVector)
	payload := []byte(plaintext)
	encryptedPayload := make([]byte, len(payload))

	for i, pByte := range payload {
		encryptedPayload[i] = key ^ pByte
		key = encryptedPayload[i] // Next key is the current encrypted byte
	}
	return encryptedPayload
}

// decrypt performs XOR decryption on the ciphertext byte array.
func decrypt(ciphertext []byte) (string, error) {
	key := byte(InitializationVector)
	decryptedPayload := make([]byte, len(ciphertext))

	for i, cByte := range ciphertext {
		decryptedPayload[i] = key ^ cByte
		key = cByte // Next key is the current ciphertext byte
	}
	return string(decryptedPayload), nil
}

// SendCommand constructs the payload, encrypts it, sends it to the device,
// receives the response, decrypts it, and unmarshals it.
func SendCommand(ip string, commandPayload interface{}) (map[string]interface{}, error) {
	// 1. Marshal the command payload to JSON string
	jsonPayloadBytes, err := json.Marshal(commandPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal command to JSON: %w", err)
	}
	jsonPayloadString := string(jsonPayloadBytes)

	// 2. Encrypt the JSON string
	encryptedRequest := encrypt(jsonPayloadString)

	// 3. Prepend the length of the *original plaintext* as a 4-byte big-endian uint32
	requestLength := uint32(len(jsonPayloadString))
	lengthPrefix := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthPrefix, requestLength)

	fullRequest := append(lengthPrefix, encryptedRequest...)

	// 4. Establish TCP connection
	address := net.JoinHostPort(ip, fmt.Sprintf("%d", DefaultPort))
	conn, err := net.DialTimeout("tcp", address, DefaultTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to device %s: %w", address, err)
	}
	defer conn.Close()

	// Set read/write deadlines
	_ = conn.SetDeadline(time.Now().Add(DefaultTimeout))

	// 5. Send the data
	_, err = conn.Write(fullRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to send data to device: %w", err)
	}

	// 6. Receive the response
	// First, read the 4-byte length prefix of the response
	responseLengthPrefix := make([]byte, 4)
	var n int // Declare n here to check if read was successful
	n, err = conn.Read(responseLengthPrefix)
	if err != nil || n < 4 {
		return nil, fmt.Errorf("failed to read response length prefix (read %d bytes): %w", n, err)
	}
	responseLength := binary.BigEndian.Uint32(responseLengthPrefix)

	// Now read the actual encrypted response
	encryptedResponse := make([]byte, responseLength)
	readBytes := 0
	for readBytes < int(responseLength) {
		nRead, err := conn.Read(encryptedResponse[readBytes:])
		if err != nil {
			return nil, fmt.Errorf("failed to read encrypted response: %w (read %d/%d bytes)", err, readBytes+nRead, responseLength)
		}
		readBytes += nRead
	}

	// 7. Decrypt the response
	decryptedResponseString, err := decrypt(encryptedResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt response: %w", err)
	}

	// 8. Unmarshal the JSON response
	var result map[string]interface{}
	err = json.Unmarshal([]byte(decryptedResponseString), &result)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON response: %w. Raw response: %s", err, decryptedResponseString)
	}

	return result, nil
}

// DiscoverDevices sends a UDP broadcast to find Kasa devices on the network.
func DiscoverDevices(timeout time.Duration) ([]DiscoveredDevice, error) {
	discoveryPayload := map[string]interface{}{
		"system": map[string]interface{}{
			"get_sysinfo": nil,
		},
	}

	payloadBytes, err := json.Marshal(discoveryPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal discovery payload: %w", err)
	}

	encryptedPayload := encrypt(string(payloadBytes))

	// Setup UDP listener
	// Listen on all interfaces, random available port
	listenAddr, err := net.ResolveUDPAddr("udp", "0.0.0.0:0")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve UDP listen address: %w", err)
	}
	conn, err := net.ListenUDP("udp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on UDP: %w", err)
	}
	defer conn.Close()

	// Prepare broadcast address
	broadcastAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("255.255.255.255:%d", DefaultPort))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve UDP broadcast address: %w", err)
	}

	// Send discovery packet
	_, err = conn.WriteToUDP(encryptedPayload, broadcastAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to send UDP discovery packet: %w", err)
	}

	fmt.Printf("Listening for Kasa devices for %.1f seconds...\n", timeout.Seconds())

	devices := make(map[string]DiscoveredDevice) // Use a map to avoid duplicates by IP
	buffer := make([]byte, 4096)                 // Buffer for responses

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)) // Short read timeout to allow loop to break
		n, remoteAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // Expected timeout, continue listening or break if overall timeout reached
			}
			// Other error, log it maybe, but continue
			fmt.Printf("Error reading UDP response: %v\n", err)
			continue
		}

		decryptedResponse, err := decrypt(buffer[:n])
		if err != nil {
			// Failed to decrypt, might not be a Kasa device or corrupted packet
			continue
		}

		var responseMap map[string]interface{}
		err = json.Unmarshal([]byte(decryptedResponse), &responseMap)
		if err != nil {
			// Failed to unmarshal JSON
			continue
		}

		// Dive into the structure: responseMap -> "system" -> "get_sysinfo"
		systemPayload, ok := responseMap["system"].(map[string]interface{})
		if !ok {
			continue
		}
		getSysinfoPayload, ok := systemPayload["get_sysinfo"].(map[string]interface{})
		if !ok {
			continue
		}

		alias, _ := getSysinfoPayload["alias"].(string)
		model, _ := getSysinfoPayload["model"].(string)
		ip := remoteAddr.IP.String()

		if alias != "" && model != "" {
			devices[ip] = DiscoveredDevice{
				IP:    ip,
				Alias: alias,
				Model: model,
			}
		}
	}

	var discoveredList []DiscoveredDevice
	for _, device := range devices {
		discoveredList = append(discoveredList, device)
	}

	return discoveredList, nil
}

// Kasa device type constants
type DeviceType string

const (
	SmartBulb     DeviceType = "IOT.SMARTBULB"
	SmartPlug     DeviceType = "IOT.SMARTPLUGSWITCH"
	UnknownDevice DeviceType = "UNKNOWN"
)

// GetDeviceTypeFromModel attempts to determine the device type from its model string.
// This is a simplified version; Kasa model strings can be complex.
func GetDeviceTypeFromModel(model string) (DeviceType, error) {
	// Simple check, can be expanded with regex or more specific model checks
	if len(model) > 0 {
		switch {
		// Example model prefixes, adjust as necessary based on observed models
		case contains(model, "LB"), contains(model, "KL"): // Light Bulbs (LB100, KL110)
			return SmartBulb, nil
		case contains(model, "HS"), contains(model, "KP"): // Smart Plugs (HS100, KP115)
			return SmartPlug, nil
		}
	}
	return UnknownDevice, fmt.Errorf("unable to determine device type from model: %s", model)
}

// contains is a helper function for string checking (case-insensitive for simplicity here, but Kasa models are usually specific)
func contains(s, substr string) bool {
	return strings.Contains(strings.ToUpper(s), strings.ToUpper(substr))
}

// SetLightState sends a command to change the light state of a Kasa bulb.
// desiredLightState should be a map, e.g., map[string]interface{}{"on_off": 1, "brightness": 50}
func SetLightState(ip string, desiredLightState map[string]interface{}) (*KasaLightState, error) {
	payload := map[string]interface{}{
		"smartlife.iot.smartbulb.lightingservice": map[string]interface{}{
			"transition_light_state": desiredLightState,
		},
	}

	response, err := SendCommand(ip, payload)
	if err != nil {
		return nil, err
	}

	// Navigate the response to find the transition_light_state object
	service, ok := response["smartlife.iot.smartbulb.lightingservice"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response structure: missing smartlife.iot.smartbulb.lightingservice")
	}

	rawState, ok := service["transition_light_state"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response structure: missing transition_light_state")
	}

	// Marshal and Unmarshal the rawState into KasaLightState struct for easier use
	stateBytes, err := json.Marshal(rawState)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal light state from response: %w", err)
	}

	var lightState KasaLightState
	if err := json.Unmarshal(stateBytes, &lightState); err != nil {
		return nil, fmt.Errorf("failed to unmarshal light state from response: %w. Raw: %s", err, string(stateBytes))
	}

	if lightState.ErrCode != 0 {
		return &lightState, fmt.Errorf("device returned error code %d while setting light state", lightState.ErrCode)
	}

	return &lightState, nil
}

// TurnOn turns the Kasa device on.
// For bulbs, it uses SetLightState. For plugs, it uses a specific plug command.
func TurnOn(ip string) (map[string]interface{}, error) {
	// First, get sysinfo to determine device type
	sysInfo, err := GetSysInfo(ip)
	if err != nil {
		return nil, fmt.Errorf("failed to get sysinfo before turning on: %w", err)
	}

	if sysInfo.MicType == string(SmartBulb) || sysInfo.IsColor == 1 || sysInfo.IsDimmable == 1 {
		_, err := SetLightState(ip, map[string]interface{}{"on_off": 1})
		if err != nil {
			return nil, err
		}
		// SetLightState already returns structured data, but for consistency with a generic TurnOn/Off if we had one
		return map[string]interface{}{"status": "success", "on_off": 1}, nil
	} else if sysInfo.MicType == string(SmartPlug) { // Assuming MicType for plugs is IOT.SMARTPLUGSWITCH
		command := map[string]interface{}{
			"system": map[string]interface{}{
				"set_relay_state": map[string]interface{}{"state": 1},
			},
		}
		return SendCommand(ip, command)
	}
	return nil, fmt.Errorf("device type '%s' (model %s) not recognized for TurnOn operation or is not a controllable light/plug", sysInfo.MicType, sysInfo.Model)
}

// TurnOff turns the Kasa device off.
// For bulbs, it uses SetLightState. For plugs, it uses a specific plug command.
func TurnOff(ip string) (map[string]interface{}, error) {
	// First, get sysinfo to determine device type
	sysInfo, err := GetSysInfo(ip)
	if err != nil {
		return nil, fmt.Errorf("failed to get sysinfo before turning off: %w", err)
	}

	if sysInfo.MicType == string(SmartBulb) || sysInfo.IsColor == 1 || sysInfo.IsDimmable == 1 {
		_, err := SetLightState(ip, map[string]interface{}{"on_off": 0})
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"status": "success", "on_off": 0}, nil
	} else if sysInfo.MicType == string(SmartPlug) { // Assuming MicType for plugs is IOT.SMARTPLUGSWITCH
		command := map[string]interface{}{
			"system": map[string]interface{}{
				"set_relay_state": map[string]interface{}{"state": 0},
			},
		}
		return SendCommand(ip, command)
	}
	return nil, fmt.Errorf("device type '%s' (model %s) not recognized for TurnOff operation or is not a controllable light/plug", sysInfo.MicType, sysInfo.Model)
}

// GetSysInfo retrieves and parses the system information from a Kasa device.
func GetSysInfo(ip string) (*KasaSystemInfo, error) {
	command := map[string]interface{}{
		"system": map[string]interface{}{
			"get_sysinfo": nil,
		},
	}

	responseMap, err := SendCommand(ip, command)
	if err != nil {
		return nil, fmt.Errorf("failed to send get_sysinfo command: %w", err)
	}

	// Convert map to JSON bytes, then unmarshal to struct
	// This is a common pattern if the initial response is already a map from SendCommand
	jsonBytes, err := json.Marshal(responseMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response map to JSON: %w", err)
	}

	var typedResponse GetSysinfoResponseWrapper
	if err := json.Unmarshal(jsonBytes, &typedResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON to GetSysinfoResponseWrapper: %w", err)
	}

	sysInfo := typedResponse.System.GetSysinfo
	if sysInfo.ErrCode != 0 {
		return &sysInfo, fmt.Errorf("Kasa device reported error for get_sysinfo - code: %d", sysInfo.ErrCode)
	}

	// If mic_mac is present and mac is not, populate mac for convenience if it's typically expected.
	if sysInfo.Mac == "" && sysInfo.MicMac != "" {
		sysInfo.Mac = sysInfo.MicMac
	}

	return &sysInfo, nil
}
