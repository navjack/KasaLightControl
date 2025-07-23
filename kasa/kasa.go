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
	DefaultPort         = 9999          // Default TCP/UDP port for Kasa device communication.
	InitializationVector = 171         // Initial byte for Kasa's XOR encryption/decryption.
	DefaultTimeout      = 5 * time.Second // Default timeout for TCP connections and responses.
	DiscoveryTimeout    = 3 * time.Second // Timeout for UDP device discovery.
)

// DiscoveredDevice holds essential information about a Kasa device found during the discovery process.
type DiscoveredDevice struct {
	IP    string `json:"ip"`    // IP address of the discovered device.
	Alias string `json:"alias"` // User-defined name (alias) of the device.
	Model string `json:"model"` // Model identifier of the device (e.g., "KL130(US)").
	// Additional fields from get_sysinfo can be added here if needed for discovery.
}

// KasaLightState represents the light state parameters of a Kasa smart bulb.
// These fields are typically found within the 'light_state' sub-object of a device's system info
// or as part of a 'transition_light_state' command/response.
type KasaLightState struct {
	OnOff      int    `json:"on_off"`      // Power state: 0 for off, 1 for on.
	Mode       string `json:"mode,omitempty"`       // Lighting mode: "normal" for white, "color" for color, "scene" for scene.
	Hue        int    `json:"hue,omitempty"`        // Hue value (0-360) for color mode.
	Saturation int    `json:"saturation,omitempty"` // Saturation value (0-100) for color mode.
	Brightness int    `json:"brightness,omitempty"` // Brightness value (0-100) for all modes.
	ColorTemp  int    `json:"color_temp,omitempty"`  // Color temperature (Kelvin) for white mode (e.g., 2500-9000).
	ErrCode    int    `json:"err_code,omitempty"`    // Error code, typically 0 for success. Present in some responses.
}

// KasaSystemInfo represents the detailed system information retrieved from a Kasa device
// via the 'get_sysinfo' command. It contains various device-specific attributes.
type KasaSystemInfo struct {
	SwVer               string          `json:"sw_ver"`                // Software version.
	HwVer               string          `json:"hw_ver"`                // Hardware version.
	Model               string          `json:"model"`                 // Device model string (e.g., "KL130(US)").
	DeviceID            string          `json:"deviceId"`              // Unique device identifier.
	OemID               string          `json:"oemId"`                 // OEM identifier.
	HwID                string          `json:"hwId"`                  // Hardware identifier.
	RSSI                int             `json:"rssi"`                  // Wi-Fi signal strength (Received Signal Strength Indication).
	LatitudeI           int             `json:"latitude_i,omitempty"`  // Latitude (integer representation), if available.
	LongitudeI          int             `json:"longitude_i,omitempty"` // Longitude (integer representation), if available.
	Alias               string          `json:"alias"`                 // User-defined name of the device.
	MicType             string          `json:"mic_type"`              // Micro-controller type, indicates device category (e.g., "IOT.SMARTBULB", "IOT.SMARTPLUGSWITCH").
	Feature             string          `json:"feature,omitempty"`     // Device features (e.g., "TIM:ENE" for timer/energy monitoring).
	Mac                 string          `json:"mac,omitempty"`         // MAC address of the device.
	MicMac              string          `json:"mic_mac,omitempty"`     // Micro-controller MAC address (often same as Mac).
	IsDimmable          int             `json:"is_dimmable,omitempty"` // 1 if dimmable, 0 otherwise.
	IsColor             int             `json:"is_color,omitempty"`    // 1 if supports color, 0 otherwise.
	IsVariableColorTemp int             `json:"is_variable_color_temp,omitempty"` // 1 if supports variable color temperature, 0 otherwise.
	LightState          *KasaLightState `json:"light_state,omitempty"` // Current light state for bulbs, pointer to allow nil for non-bulb devices.
	RelayState          int             `json:"relay_state,omitempty"` // Current relay state for smart plugs (0=off, 1=on).
	ErrCode             int             `json:"err_code"`              // Error code, typically 0 for success.
}

// GetSysinfoResponseWrapper is a helper struct used to unmarshal the nested JSON response
// from a 'get_sysinfo' command. It provides direct access to the KasaSystemInfo.
type GetSysinfoResponseWrapper struct {
	System struct {
		GetSysinfo KasaSystemInfo `json:"get_sysinfo"` // Contains the detailed system information.
	} `json:"system"`
}

// TransitionLightStateResponseWrapper is a helper struct used to unmarshal the nested JSON response
// from a 'transition_light_state' command. It provides direct access to the KasaLightState.
type TransitionLightStateResponseWrapper struct {
	SmartlifeIoTSmartbulbLightingservice struct {
		TransitionLightState KasaLightState `json:"transition_light_state"` // Contains the updated light state.
	} `json:"smartlife.iot.smartbulb.lightingservice"`
}

// encrypt performs XOR encryption on the plaintext string according to the Kasa protocol.
// The encryption uses a changing key: the initial key is InitializationVector,
// and subsequent keys are the previously encrypted byte.
// This function only returns the encrypted payload; the 4-byte length prefixing
// (length of the original plaintext) is handled by the SendCommand function.
func encrypt(plaintext string) []byte {
	key := byte(InitializationVector) // Start with the predefined initialization vector.
	payload := []byte(plaintext)       // Convert the plaintext string to a byte slice.
	encryptedPayload := make([]byte, len(payload)) // Prepare a slice for the encrypted bytes.

	// Iterate through each byte of the payload to perform XOR encryption.
	for i, pByte := range payload {
		encryptedPayload[i] = key ^ pByte // XOR the current plaintext byte with the current key.
		key = encryptedPayload[i]         // Update the key for the next byte to the current encrypted byte.
	}
	return encryptedPayload
}

// decrypt performs XOR decryption on the ciphertext byte array according to the Kasa protocol.
// The decryption process mirrors encryption, using the previous ciphertext byte as the next key.
func decrypt(ciphertext []byte) (string, error) {
	key := byte(InitializationVector) // Start with the predefined initialization vector.
	decryptedPayload := make([]byte, len(ciphertext)) // Prepare a slice for the decrypted bytes.

	// Iterate through each byte of the ciphertext to perform XOR decryption.
	for i, cByte := range ciphertext {
		decryptedPayload[i] = key ^ cByte // XOR the current ciphertext byte with the current key.
		key = cByte                       // Update the key for the next byte to the current ciphertext byte.
	}
	return string(decryptedPayload), nil // Convert the decrypted byte slice back to a string.
}

// SendCommand constructs the full Kasa command payload (length prefix + encrypted data),
// sends it to the specified Kasa device via TCP, receives the encrypted response,
// decrypts it, and unmarshals the JSON response into a map.
func SendCommand(ip string, commandPayload interface{}) (map[string]interface{}, error) {
	// 1. Marshal the command payload (Go interface{}) into a JSON string.
	jsonPayloadBytes, err := json.Marshal(commandPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal command to JSON: %w", err)
	}
	jsonPayloadString := string(jsonPayloadBytes)

	// 2. Encrypt the JSON string using the Kasa XOR encryption algorithm.
	encryptedRequest := encrypt(jsonPayloadString)

	// 3. Prepend the length of the *original plaintext* (JSON string) as a 4-byte big-endian unsigned 32-bit integer.
	// This length prefix is a crucial part of the Kasa protocol for framing messages.
	requestLength := uint32(len(jsonPayloadString))
	lengthPrefix := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthPrefix, requestLength)

	// Combine the length prefix and the encrypted payload to form the full request.
	fullRequest := append(lengthPrefix, encryptedRequest...)

	// 4. Establish a TCP connection to the Kasa device on the default port.
	address := net.JoinHostPort(ip, fmt.Sprintf("%d", DefaultPort))
	conn, err := net.DialTimeout("tcp", address, DefaultTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to device %s: %w", address, err)
	}
	defer conn.Close() // Ensure the connection is closed when the function exits.

	// Set a read/write deadline for the connection to prevent indefinite blocking.
	_ = conn.SetDeadline(time.Now().Add(DefaultTimeout))

	// 5. Send the full request (length prefix + encrypted payload) over the TCP connection.
	_, err = conn.Write(fullRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to send data to device: %w", err)
	}

	// 6. Receive the response from the device.
	// First, read the 4-byte length prefix of the incoming encrypted response.
	responseLengthPrefix := make([]byte, 4)
	var n int // Variable to store the number of bytes read.
	n, err = conn.Read(responseLengthPrefix)
	if err != nil || n < 4 {
		return nil, fmt.Errorf("failed to read response length prefix (read %d bytes): %w", n, err)
	}
	responseLength := binary.BigEndian.Uint32(responseLengthPrefix)

	// Now, read the actual encrypted response payload based on the received length.
	encryptedResponse := make([]byte, responseLength)
	readBytes := 0
	for readBytes < int(responseLength) {
		nRead, err := conn.Read(encryptedResponse[readBytes:])
		if err != nil {
			return nil, fmt.Errorf("failed to read encrypted response: %w (read %d/%d bytes)", err, readBytes+nRead, responseLength)
		}
		readBytes += nRead
	}

	// 7. Decrypt the received encrypted response payload.
	decryptedResponseString, err := decrypt(encryptedResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt response: %w", err)
	}

	// 8. Unmarshal the decrypted JSON string into a generic map.
	var result map[string]interface{}
	err = json.Unmarshal([]byte(decryptedResponseString), &result)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON response: %w. Raw response: %s", err, decryptedResponseString)
	}

	return result, nil // Return the unmarshaled JSON response.
}

// DiscoverDevices sends a UDP broadcast packet to the local network to find Kasa smart devices.
// It listens for responses for a specified duration and returns a list of discovered devices.
func DiscoverDevices(timeout time.Duration) ([]DiscoveredDevice, error) {
	// The discovery command is a simple 'get_sysinfo' request encapsulated in the Kasa protocol.
	discoveryPayload := map[string]interface{}{
		"system": map[string]interface{}{
			"get_sysinfo": nil, // Requesting system information from all devices.
		},
	}

	payloadBytes, err := json.Marshal(discoveryPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal discovery payload: %w", err)
	}

	encryptedPayload := encrypt(string(payloadBytes))

	// 1. Set up a UDP listener on all available interfaces and a random ephemeral port.
	listenAddr, err := net.ResolveUDPAddr("udp", "0.0.0.0:0")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve UDP listen address: %w", err)
	}
	conn, err := net.ListenUDP("udp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on UDP: %w", err)
	}
	defer conn.Close() // Ensure the UDP connection is closed when the function exits.

	// 2. Prepare the broadcast address for sending the discovery packet.
	broadcastAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("255.255.255.255:%d", DefaultPort))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve UDP broadcast address: %w", err)
	}

	// 3. Send the encrypted discovery packet to the broadcast address.
	_, err = conn.WriteToUDP(encryptedPayload, broadcastAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to send UDP discovery packet: %w", err)
	}

	// 4. Listen for UDP responses from Kasa devices for the specified timeout duration.
	fmt.Printf("Listening for Kasa devices for %.1f seconds...\n", timeout.Seconds())

	devices := make(map[string]DiscoveredDevice) // Use a map to store discovered devices, keyed by IP to avoid duplicates.
	buffer := make([]byte, 4096)                 // Buffer to read incoming UDP packets.

	deadline := time.Now().Add(timeout) // Calculate the time when listening should stop.
	for time.Now().Before(deadline) {   // Loop until the overall timeout is reached.
		// Set a short read deadline for each ReadFromUDP call to prevent blocking indefinitely
		// and allow the loop to check the overall deadline.
		conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, remoteAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			// If the error is a timeout, continue to the next iteration to check the overall deadline.
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			// Log other types of errors but continue listening.
			fmt.Printf("Error reading UDP response: %v\n", err)
			continue
		}

		// Attempt to decrypt the received UDP packet.
		decryptedResponse, err := decrypt(buffer[:n])
		if err != nil {
			// If decryption fails, it's likely not a Kasa device response or the packet is corrupted; skip.
			continue
		}

		// Attempt to unmarshal the decrypted response into a generic map.
		var responseMap map[string]interface{}
		err = json.Unmarshal([]byte(decryptedResponse), &responseMap)
		if err != nil {
			// If unmarshaling fails, the response is not valid JSON; skip.
			continue
		}

		// Extract system information from the nested JSON structure.
		// The Kasa 'get_sysinfo' response is typically structured as {"system": {"get_sysinfo": {...}}}.
		systemPayload, ok := responseMap["system"].(map[string]interface{})
		if !ok {
			continue // Not a valid system response.
		}
		getSysinfoPayload, ok := systemPayload["get_sysinfo"].(map[string]interface{})
		if !ok {
			continue // Missing 'get_sysinfo' payload.
		}

		// Extract alias, model, and IP address from the system info.
		alias, _ := getSysinfoPayload["alias"].(string)
		model, _ := getSysinfoPayload["model"].(string)
		ip := remoteAddr.IP.String()

		// If both alias and model are present, consider it a valid Kasa device and add to the map.
		if alias != "" && model != "" {
			devices[ip] = DiscoveredDevice{
				IP:    ip,
				Alias: alias,
				Model: model,
			}
		}
	}

	// Convert the map of unique discovered devices into a slice for return.
	var discoveredList []DiscoveredDevice
	for _, device := range devices {
		discoveredList = append(discoveredList, device)
	}

	return discoveredList, nil
}

// DeviceType represents the categorized type of a Kasa smart device.
type DeviceType string

const (
	SmartBulb     DeviceType = "IOT.SMARTBULB"     // Represents a Kasa smart bulb.
	SmartPlug     DeviceType = "IOT.SMARTPLUGSWITCH" // Represents a Kasa smart plug.
	UnknownDevice DeviceType = "UNKNOWN"         // Represents an unrecognized or unhandled device type.
)

// GetDeviceTypeFromModel attempts to determine the general type of a Kasa device
// based on its model string. This function provides a simplified classification.
// More complex logic might be needed for a comprehensive model-to-type mapping.
func GetDeviceTypeFromModel(model string) (DeviceType, error) {
	// Perform a case-insensitive check for common model prefixes to identify device types.
	if len(model) > 0 {
		switch {
		case contains(model, "LB"), contains(model, "KL"): // "LB" and "KL" typically indicate Light Bulbs (e.g., LB100, KL130).
			return SmartBulb, nil
		case contains(model, "HS"), contains(model, "KP"): // "HS" and "KP" typically indicate Smart Plugs (e.g., HS100, KP115).
			return SmartPlug, nil
		}
	}
	// If no known model prefix is found, return UnknownDevice with an error.
	return UnknownDevice, fmt.Errorf("unable to determine device type from model: %s", model)
}

// contains is a helper function for string checking (case-insensitive for simplicity here, but Kasa models are usually specific)
func contains(s, substr string) bool {
	return strings.Contains(strings.ToUpper(s), strings.ToUpper(substr))
}

// SetLightState sends a command to a Kasa smart bulb to change its light state.
// The `desiredLightState` parameter should be a map containing the light properties to set,
// such as "on_off" (0 or 1), "brightness" (0-100), "hue" (0-360), "saturation" (0-100),
// or "color_temp" (e.g., 2500-9000).
// This function uses the `transition_light_state` command.
func SetLightState(ip string, desiredLightState map[string]interface{}) (*KasaLightState, error) {
	// Construct the full command payload, encapsulating the desired light state.
	payload := map[string]interface{}{
		"smartlife.iot.smartbulb.lightingservice": map[string]interface{}{
			"transition_light_state": desiredLightState, // The actual light state parameters.
		},
	}

	// Send the command to the device and receive the response.
	response, err := SendCommand(ip, payload)
	if err != nil {
		return nil, err
	}

	// Parse the response to extract the light state. The response structure is nested:
	// `response["smartlife.iot.smartbulb.lightingservice"]["transition_light_state"]`
	service, ok := response["smartlife.iot.smartbulb.lightingservice"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response structure: missing smartlife.iot.smartbulb.lightingservice in response")
	}

	rawState, ok := service["transition_light_state"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response structure: missing transition_light_state in response")
	}

	// Marshal the raw light state map back into JSON bytes and then unmarshal it into a KasaLightState struct.
	// This ensures proper type conversion and validation of the response data.
	stateBytes, err := json.Marshal(rawState)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal light state from response: %w", err)
	}

	var lightState KasaLightState
	if err := json.Unmarshal(stateBytes, &lightState); err != nil {
		return nil, fmt.Errorf("failed to unmarshal light state from response: %w. Raw: %s", err, string(stateBytes))
	}

	// Check for any error code returned by the device.
	if lightState.ErrCode != 0 {
		return &lightState, fmt.Errorf("device returned error code %d while setting light state", lightState.ErrCode)
	}

	return &lightState, nil // Return the successfully updated light state.
}

// TurnOn sends a command to turn on a Kasa smart device.
// It first retrieves system information to determine if the device is a smart bulb or a smart plug,
// then sends the appropriate command (`SetLightState` for bulbs, `set_relay_state` for plugs).
func TurnOn(ip string) (map[string]interface{}, error) {
	// Retrieve system information to identify the device type and capabilities.
	sysInfo, err := GetSysInfo(ip)
	if err != nil {
		return nil, fmt.Errorf("failed to get sysinfo before turning on: %w", err)
	}

	// Check if the device is a smart bulb (based on MicType or light capabilities).
	if sysInfo.MicType == string(SmartBulb) || sysInfo.IsColor == 1 || sysInfo.IsDimmable == 1 {
		// For smart bulbs, use the SetLightState function to turn on.
		_, err := SetLightState(ip, map[string]interface{}{"on_off": 1})
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"status": "success", "on_off": 1}, nil // Return success status.
	} else if sysInfo.MicType == string(SmartPlug) { // Check if the device is a smart plug.
		// For smart plugs, send a 'set_relay_state' command with state 1 (on).
		command := map[string]interface{}{
			"system": map[string]interface{}{
				"set_relay_state": map[string]interface{}{"state": 1},
			},
		}
		return SendCommand(ip, command) // Send the command and return its response.
	}
	// If the device type is not recognized or supported for this operation, return an error.
	return nil, fmt.Errorf("device type '%s' (model %s) not recognized for TurnOn operation or is not a controllable light/plug", sysInfo.MicType, sysInfo.Model)
}

// TurnOff sends a command to turn off a Kasa smart device.
// Similar to TurnOn, it first determines the device type and sends the appropriate command.
func TurnOff(ip string) (map[string]interface{}, error) {
	// Retrieve system information to identify the device type and capabilities.
	sysInfo, err := GetSysInfo(ip)
	if err != nil {
		return nil, fmt.Errorf("failed to get sysinfo before turning off: %w", err)
	}

	// Check if the device is a smart bulb.
	if sysInfo.MicType == string(SmartBulb) || sysInfo.IsColor == 1 || sysInfo.IsDimmable == 1 {
		// For smart bulbs, use the SetLightState function to turn off.
		_, err := SetLightState(ip, map[string]interface{}{"on_off": 0})
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"status": "success", "on_off": 0}, nil // Return success status.
	} else if sysInfo.MicType == string(SmartPlug) { // Check if the device is a smart plug.
		// For smart plugs, send a 'set_relay_state' command with state 0 (off).
		command := map[string]interface{}{
			"system": map[string]interface{}{
				"set_relay_state": map[string]interface{}{"state": 0},
			},
		}
		return SendCommand(ip, command) // Send the command and return its response.
	}
	// If the device type is not recognized or supported for this operation, return an error.
	return nil, fmt.Errorf("device type '%s' (model %s) not recognized for TurnOff operation or is not a controllable light/plug", sysInfo.MicType, sysInfo.Model)
}

// GetSysInfo retrieves and parses the detailed system information from a Kasa device.
// It sends a 'get_sysinfo' command and unmarshals the response into a KasaSystemInfo struct.
// GetSysInfo retrieves and parses the detailed system information from a Kasa device.
// It sends a 'get_sysinfo' command and unmarshals the response into a KasaSystemInfo struct.
func GetSysInfo(ip string) (*KasaSystemInfo, error) {
	// Construct the command payload to request system information.
	command := map[string]interface{}{
		"system": map[string]interface{}{
			"get_sysinfo": nil, // Requesting all available system information.
		},
	}

	// Send the command to the device and receive the response.
	response, err := SendCommand(ip, command)
	if err != nil {
		return nil, err
	}

	// Unmarshal the raw response into the GetSysinfoResponseWrapper struct.
	// This helper struct is used to correctly parse the nested JSON structure.
	var sysinfoWrapper GetSysinfoResponseWrapper
	jsonString, _ := json.Marshal(response) // Re-marshal to JSON string to unmarshal into specific struct.
	err = json.Unmarshal(jsonString, &sysinfoWrapper)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal sysinfo response: %w", err)
	}

	// Check for any error code returned by the device within the system info.
	if sysinfoWrapper.System.GetSysinfo.ErrCode != 0 {
		return nil, fmt.Errorf("device returned error code %d for get_sysinfo", sysinfoWrapper.System.GetSysinfo.ErrCode)
	}

	return &sysinfoWrapper.System.GetSysinfo, nil // Return the extracted KasaSystemInfo struct.
}
