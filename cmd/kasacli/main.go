package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/user/kasalightcontrol/kasa"
)

func main() {
	bulbIP := flag.String("ip", "", "IP address of the Kasa bulb (required for most commands)")
	commandStr := flag.String("command", "get_sysinfo", "Command to execute: get_sysinfo, set_hsv, turn_on, turn_off, set_brightness, set_colortemp, discover")

	// Flags for set_hsv
	hue := flag.Int("hue", -1, "Hue (0-360) for set_hsv")
	sat := flag.Int("sat", -1, "Saturation (0-100) for set_hsv")
	val := flag.Int("val", -1, "Value/Brightness (0-100) for set_hsv or brightness for set_brightness")

	// Flag for set_brightness (uses -val flag)

	// Flag for set_colortemp
	kelvin := flag.Int("kelvin", -1, "Color temperature in Kelvin (e.g., 2700, 4000, 6500) for set_colortemp")

	transition := flag.Int("transition", 0, "Transition period in milliseconds (optional)")

	flag.Parse()

	if *commandStr != "discover" && *bulbIP == "" {
		fmt.Println("Error: Bulb IP address is required for this command. Use the -ip flag.")
		flag.Usage()
		os.Exit(1)
	}

	var actionDescription string
	var desiredLightState map[string]interface{} // To hold the state for SetLightState func

	switch *commandStr {
	case "get_sysinfo":
		actionDescription = fmt.Sprintf("Attempting to get system info from bulb at %s...", *bulbIP)
		fmt.Println(actionDescription)
		sysInfo, err := kasa.GetSysInfo(*bulbIP)
		if err != nil {
			fmt.Printf("Error getting system info: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Successfully retrieved system info:")
		fmt.Printf("  Alias: %s\n", sysInfo.Alias)
		fmt.Printf("  Model: %s (HW: %s, SW: %s)\n", sysInfo.Model, sysInfo.HwVer, sysInfo.SwVer)
		fmt.Printf("  Device ID: %s\n", sysInfo.DeviceID)
		fmt.Printf("  MAC Address: %s\n", sysInfo.Mac)
		fmt.Printf("  RSSI: %d\n", sysInfo.RSSI)
		if sysInfo.LightState != nil {
			fmt.Println("  Light State:")
			fmt.Printf("    On: %t\n", sysInfo.LightState.OnOff == 1)
			if sysInfo.IsColor == 1 {
				fmt.Printf("    Mode: %s, Hue: %d, Saturation: %d, Brightness: %d\n", sysInfo.LightState.Mode, sysInfo.LightState.Hue, sysInfo.LightState.Saturation, sysInfo.LightState.Brightness)
			}
			if sysInfo.IsVariableColorTemp == 1 {
				fmt.Printf("    Color Temp: %dK\n", sysInfo.LightState.ColorTemp)
			}
			// If only dimmable and not color/temp (e.g. some white bulbs)
			if sysInfo.IsColor != 1 && sysInfo.IsVariableColorTemp != 1 && sysInfo.IsDimmable == 1 {
				fmt.Printf("    Brightness: %d\n", sysInfo.LightState.Brightness)
			}
		}
		// No need to call SendCommand for get_sysinfo as GetSysInfo handles it.
		return // Exit after handling get_sysinfo
	case "set_hsv":
		if *hue == -1 || *sat == -1 || *val == -1 {
			fmt.Println("Error: For set_hsv, -hue, -sat, and -val flags are required.")
			flag.Usage()
			os.Exit(1)
		}
		if *hue < 0 || *hue > 360 {
			fmt.Println("Error: Hue must be between 0 and 360.")
			os.Exit(1)
		}
		if *sat < 0 || *sat > 100 {
			fmt.Println("Error: Saturation must be between 0 and 100.")
			os.Exit(1)
		}
		if *val < 0 || *val > 100 {
			fmt.Println("Error: Value/Brightness must be between 0 and 100.")
			os.Exit(1)
		}
		actionDescription = fmt.Sprintf("Attempting to set HSV (H:%d, S:%d, V:%d, T:%dms) on bulb at %s...", *hue, *sat, *val, *transition, *bulbIP)
		desiredLightState = map[string]interface{}{
			"on_off":         1, // Ensure bulb is on when setting color
			"ignore_default": 1,
			"hue":            *hue,
			"saturation":     *sat,
			"brightness":     *val,
			"color_temp":     0,
		}
		if *transition > 0 {
			desiredLightState["transition_period"] = *transition
		}
	case "turn_on":
		actionDescription = fmt.Sprintf("Attempting to turn ON bulb at %s (T:%dms)...", *bulbIP, *transition)
		desiredLightState = map[string]interface{}{
			"on_off":         1,
			"ignore_default": 1,
		}
		if *transition > 0 {
			desiredLightState["transition_period"] = *transition
		}
	case "turn_off":
		actionDescription = fmt.Sprintf("Attempting to turn OFF bulb at %s (T:%dms)...", *bulbIP, *transition)
		desiredLightState = map[string]interface{}{
			"on_off":         0,
			"ignore_default": 1,
		}
		if *transition > 0 {
			desiredLightState["transition_period"] = *transition
		}
	case "set_brightness":
		if *val == -1 {
			fmt.Println("Error: For set_brightness, the -val flag (0-100) is required.")
			flag.Usage()
			os.Exit(1)
		}
		if *val < 0 || *val > 100 {
			fmt.Println("Error: Value/Brightness must be between 0 and 100.")
			os.Exit(1)
		}
		actionDescription = fmt.Sprintf("Attempting to set brightness to %d%% (T:%dms) on bulb at %s...", *val, *transition, *bulbIP)
		desiredLightState = map[string]interface{}{
			"on_off":         1,
			"ignore_default": 1,
			"brightness":     *val,
		}
		if *transition > 0 {
			desiredLightState["transition_period"] = *transition
		}
	case "set_colortemp":
		if *kelvin == -1 {
			fmt.Println("Error: For set_colortemp, the -kelvin flag is required.")
			flag.Usage()
			os.Exit(1)
		}
		if *kelvin < 2000 || *kelvin > 9000 {
			fmt.Println("Error: Kelvin must be within a reasonable range (e.g., 2000-9000).")
			os.Exit(1)
		}
		valForTemp := 100
		if *val != -1 {
			if *val < 0 || *val > 100 {
				fmt.Println("Error: Value/Brightness for color temperature must be between 0 and 100.")
				os.Exit(1)
			}
			valForTemp = *val
		}
		actionDescription = fmt.Sprintf("Attempting to set color temperature to %dK at %d%% brightness (T:%dms) on bulb at %s...", *kelvin, valForTemp, *transition, *bulbIP)
		desiredLightState = map[string]interface{}{
			"on_off":         1,
			"ignore_default": 1,
			"color_temp":     *kelvin,
			"brightness":     valForTemp,
			"hue":            0,
			"saturation":     0,
		}
		if *transition > 0 {
			desiredLightState["transition_period"] = *transition
		}
	case "discover":
		actionDescription = "Attempting to discover Kasa devices on the network..."
		fmt.Println(actionDescription) // Print action description before potential long operation
		discoveredDevices, err := kasa.DiscoverDevices(kasa.DiscoveryTimeout) // Using default from kasa package
		if err != nil {
			fmt.Printf("Error during device discovery: %v\n", err)
			os.Exit(1)
		}
		if len(discoveredDevices) == 0 {
			fmt.Println("No Kasa devices found.")
		} else {
			fmt.Printf("Found %d Kasa device(s):\n", len(discoveredDevices))
			for i, device := range discoveredDevices {
				fmt.Printf("  %d. IP: %s, Alias: %s, Model: %s\n", i+1, device.IP, device.Alias, device.Model)
			}
		}
		return // Discovery command doesn't send a command to a specific IP, so we exit here.
	default:
		fmt.Printf("Error: Unknown command '%s'. Valid commands are 'get_sysinfo', 'set_hsv', 'turn_on', 'turn_off', 'set_brightness', 'set_colortemp', or 'discover'.\n", *commandStr)
		flag.Usage()
		os.Exit(1)
	}

	// Only run SendCommand if it's not a discover or get_sysinfo (already handled) command
	fmt.Println(actionDescription)

	// Call SetLightState for commands that modify the light state
	returnedLightState, err := kasa.SetLightState(*bulbIP, desiredLightState)
	if err != nil {
		fmt.Printf("Error setting light state: %v\n", err)
		if returnedLightState != nil && returnedLightState.ErrCode != 0 {
			// Kasa device might have returned an error code that SetLightState also wrapped
			fmt.Printf("  Kasa device error code: %d\n", returnedLightState.ErrCode)
		}
		os.Exit(1)
	}

	fmt.Println("Successfully set light state. Current state from device:")
	fmt.Printf("  On: %t\n", returnedLightState.OnOff == 1)
	fmt.Printf("  Mode: %s\n", returnedLightState.Mode)
	fmt.Printf("  Brightness: %d\n", returnedLightState.Brightness)
	if returnedLightState.ColorTemp > 0 {
		fmt.Printf("  Color Temp: %dK\n", returnedLightState.ColorTemp)
		fmt.Printf("  Hue: %d, Saturation: %d (should be 0 if color temp is active)\n", returnedLightState.Hue, returnedLightState.Saturation)
	} else {
		fmt.Printf("  Hue: %d, Saturation: %d\n", returnedLightState.Hue, returnedLightState.Saturation)
	}
	if returnedLightState.ErrCode != 0 { // Should be caught by SetLightState, but good for verbosity
		fmt.Printf("  Device reported error code: %d\n", returnedLightState.ErrCode)
	}
}
