package kasa

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
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
