# Kasa Light Control Application

## Project Goal
This application provides a GUI-based solution for controlling TPLink Kasa smart bulbs locally, fully replicating the functionality of an existing PowerShell script and eliminating the dependency on the Python-Kasa framework.

## Project Description
This project successfully implements the network protocols and communication methods used by TPLink Kasa devices directly in GoLang. Developed with cross-platform capabilities in mind, it provides a robust and reliable solution for local control of Kasa smart bulbs.

## Implemented Features:

*   **Network Protocol Implementation**: Successfully reverse-engineered and implemented network protocols for Kasa devices in GoLang.
*   **Full Local Control**: Comprehensive control capabilities for Kasa smart bulbs, including:
    *   Turning devices on/off.
    *   Adjusting brightness, color temperature, and color settings.
    *   Retrieving device state.
*   **Device Discovery**: Automatic discovery of Kasa smart devices on the local network.
*   **User-Friendly GUI**: A clean, intuitive, and responsive browser-based graphical user interface.

## Usage

### Prerequisites
- Go (Golang) installed on your system.

### Running the Server
1.  Navigate to the project's root directory (`KasaLightControl`).
2.  Run the server using the command:
    ```bash
    go run cmd/kasaserver/main.go
    ```
3.  The server will start, and it should automatically open the web UI in your default browser at `http://localhost:8080`.
4.  If it doesn't open automatically, you can manually navigate to `http://localhost:8080` in your web browser.

### Using the Web UI
- **Discover Devices**: Click the "Discover Devices" button to scan your local network for Kasa smart devices.
- **View Device Details**: Click on a device in the list to see its details and control panel.
- **Control Lights**:
    - Toggle power on/off.
    - Adjust brightness using the slider.
    - Set color using the color picker (for color bulbs).
    - Adjust color temperature using the temperature slider (for tunable white/color bulbs).
- **Natural Light Sync**: 
    - For compatible lights (color or tunable white), a "Natural Light Sync" toggle is available in the device details panel.
    - When enabled, this feature automatically adjusts the bulb's brightness and color temperature throughout the day to mimic natural sunlight patterns.
    - Toggle the switch to enable or disable this feature for the selected device.
- **Shutdown Server**: Click the "Shutdown Server" button (usually red) to gracefully stop the backend server. You will be prompted for confirmation. After shutdown, the web UI will no longer be responsive.

## API Endpoints

The Kasa Light Control server exposes the following RESTful API endpoints:

- **`GET /api/discover`**
  - Triggers a network scan for Kasa devices.
  - **Response**: JSON array of discovered devices.
    ```json
    [
      {
        "deviceId": "string",
        "model": "string",
        "alias": "string",
        "ip": "string",
        // ... other fields ...
      }
    ]
    ```

- **`GET /api/device/{ip}/details`**
  - Fetches detailed information and current state for a specific device by its IP address.
  - **Response**: JSON object with device details.
    ```json
    {
      "deviceId": "string",
      "model": "string",
      "alias": "string",
      "ip": "string",
      "macAddress": "string",
      "isColor": true,
      "isDimmer": true,
      "isVariableColorTemperature": true,
      "powerState": true,
      "brightness": 50,
      "colorTemp": 4000,
      "hue": 120,
      "saturation": 100,
      "isNaturalLightActive": false,
      "status": "Online"
    }
    ```

- **`POST /api/device/{ip}/power`**
  - Turns a device on or off.
  - **Request Body**: `{"state": int}` (1 for on, 0 for off)
  - **Response**: Text message indicating success or failure.

- **`POST /api/device/{ip}/light-state`**
  - Sets the light state for a bulb (brightness, color, temperature).
  - **Request Body**: 
    ```json
    {
      "on_off": int, // Optional, 1 for on, 0 for off
      "brightness": int, // Optional, 0-100
      "color_temp": int, // Optional, Kelvin (e.g., 2500-9000 for KL130 bulbs)
      "hue": int,        // Optional, 0-360
      "saturation": int  // Optional, 0-100
    }
    ```
  - **Response**: Text message indicating success or failure.

- **`POST /api/device/{ip}/natural-light`**
  - Enables or disables Natural Light Sync for a device.
  - **Request Body**: `{"enable": boolean}` (true to enable, false to disable)
  - **Response**: Text message indicating success or failure.

- **`POST /api/shutdown`**
  - Gracefully shuts down the Kasa Light Control server.
  - **Response**: Text message "Server is shutting down..."
