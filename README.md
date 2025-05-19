# Kasa Light Control Application

## Project Goal
Develop a GUI-based application that fully replicates the functionality of an existing PowerShell script for controlling TPLink Kasa smart bulbs locally, eliminating the dependency on the Python-Kasa framework.

## Project Description
This project aims to understand the protocol and communication methods employed by TPLink Kasa devices (as potentially used by libraries like Python-Kasa) and to independently implement this functionality directly in GoLang. The initial development will focus on GoLang for its cross-platform capabilities, with potential future ports or expansions into Swift for native macOS GUI integration.

## Key Features to Implement:

*   Reverse-engineer and document the network protocols used by Kasa devices.
*   Full local control capabilities including:
    *   Turning devices on/off.
    *   Adjusting brightness, color temperature, and color settings.
    *   Retrieving device state and power usage (where applicable).
*   Device discovery on the local network.
*   Historical energy usage monitoring (for applicable devices).
*   A clean, user-friendly GUI.

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
- **Shutdown Server**: Click the "Shutdown Server" button (usually red) to gracefully stop the backend server. You will be prompted for confirmation. After shutdown, the web UI will no longer be responsive.
