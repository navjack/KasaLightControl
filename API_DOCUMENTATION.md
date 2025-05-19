# Kasa Light Control - API Documentation

## Base URL
All API endpoints are relative to the following base URL:
`http://localhost:8080`

---

## Endpoints

### 1. Discover Devices
- **Method**: `GET`
- **Path**: `/api/discover`
- **Description**: Scans the local network for Kasa smart devices and returns a list of discovered devices.
- **Query Parameters**: None
- **Request Body**: None
- **Response Body**:
  - **Type**: `application/json`
  - **Structure**: Array of `Device` objects.
    ```json
    [
      {
        "deviceId": "string_device_id_optional",
        "alias": "My Smart Bulb",
        "model": "KL130(US)",
        "ip": "192.168.1.100",
        "mac_address": "AA:BB:CC:DD:EE:FF",
        "is_color": true,
        "is_dimmable": true,
        "relay_state": true, // true if on, false if off
        "status": "Discovered" 
        // hue, saturation, brightness, color_temp are not populated by discovery
      }
      // ... more devices
    ]
    ```

### 2. Get Device Details
- **Method**: `GET`
- **Path**: `/api/device-details`
- **Description**: Retrieves detailed information and current state for a specific Kasa device.
- **Query Parameters**:
  - `ip` (string, required): The IP address of the Kasa device.
    Example: `/api/device-details?ip=192.168.1.100`
- **Request Body**: None
- **Response Body**:
  - **Type**: `application/json`
  - **Structure**: A single `Device` object.
    ```json
    {
      "deviceId": "string_device_id",
      "alias": "Living Room Lamp",
      "model": "KL130(US)",
      "ip": "192.168.1.100",
      "mac_address": "AA:BB:CC:DD:EE:FF",
      "is_color": true,
      "is_dimmable": true,
      "relay_state": true, // true if on, false if off
      "status": "Online",
      "hue": 120,        // 0-360
      "saturation": 80,  // 0-100
      "brightness": 50,  // 0-100
      "color_temp": 3500 // 2500-6500 (Kelvin), present if supported
    }
    ```
  - **Error Response Example (if device fetch fails)**:
    ```json
    {
        "error": "Failed to get device system info: <error_details>"
    }
    ```
    (Status Code: 500 Internal Server Error)

### 3. Set Device Power
- **Method**: `POST`
- **Path**: `/api/set-power`
- **Description**: Turns a Kasa device on or off.
- **Request Body**:
  - **Type**: `application/json`
  - **Structure**:
    ```json
    {
      "ip": "string_device_ip", // Required
      "on": boolean             // Required, true for on, false for off
    }
    ```
  - **Example**:
    ```json
    {
      "ip": "192.168.1.100",
      "on": true
    }
    ```
- **Response Body**:
  - **Type**: `application/json`
  - **Success Example**:
    ```json
    {
      "status": "success",
      "message": "Power state set for <device_alias> (IP: <device_ip>)"
    }
    ```
  - **Error Example**:
    ```json
    {
        "status": "error",
        "message": "Failed to set power state: <error_details>"
    }
    ```

### 4. Set Light State
- **Method**: `POST`
- **Path**: `/api/set-light-state`
- **Description**: Sets the light state of a Kasa smart bulb (brightness, hue, saturation, color temperature, power).
- **Request Body**:
  - **Type**: `application/json`
  - **Structure**:
    ```json
    {
      "ip": "string_device_ip",        // Required
      "on_off": boolean,               // Optional, true for on, false for off
      "brightness": int,             // Optional, 0-100
      "hue": int,                    // Optional, 0-360
      "saturation": int,             // Optional, 0-100
      "color_temp": int              // Optional, 2500-6500 (Kelvin). Setting color_temp will typically override hue/saturation.
    }
    ```
    *Note: Send only the parameters you wish to change. If `color_temp` is set to a non-zero value, the bulb usually switches to white mode, and `hue`/`saturation` might be ignored or reset.*
  - **Example (set color and brightness)**:
    ```json
    {
      "ip": "192.168.1.100",
      "on_off": true,
      "hue": 240,
      "saturation": 100,
      "brightness": 75
    }
    ```
  - **Example (set color temperature)**:
    ```json
    {
      "ip": "192.168.1.100",
      "on_off": true,
      "color_temp": 4000,
      "brightness": 60
    }
    ```
- **Response Body**:
  - **Type**: `application/json`
  - **Success Example**:
    ```json
    {
      "status": "success",
      "message": "Light state set for <device_alias> (IP: <device_ip>)"
    }
    ```
  - **Error Example**:
    ```json
    {
        "status": "error",
        "message": "Failed to set light state: <error_details>"
    }
    ```

### 5. Shutdown Server
- **Method**: `POST` (or `GET`)
- **Path**: `/api/shutdown`
- **Description**: Gracefully shuts down the Kasa Light Control server.
- **Query Parameters**: None
- **Request Body**: None
- **Response Body**:
  - **Type**: `text/plain`
  - **Content**: `Server is shutting down...`
  (Status Code: 200 OK)
