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

	var commandPayload interface{}
	var actionDescription string

	switch *commandStr {
	case "get_sysinfo":
		actionDescription = fmt.Sprintf("Attempting to get system info from bulb at %s...", *bulbIP)
		commandPayload = map[string]interface{}{
			"system": map[string]interface{}{
				"get_sysinfo": nil,
			},
		}
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
		state := map[string]interface{}{
			"on_off":         1, // Ensure bulb is on when setting color
			"ignore_default": 1,
			"hue":            *hue,
			"saturation":     *sat,
			"brightness":     *val, // Kasa API uses 'brightness' for the V in HSV
			"color_temp":     0,    // Must be 0 for HSV mode
		}
		if *transition > 0 {
			state["transition_period"] = *transition
		}
		commandPayload = map[string]interface{}{
			"smartlife.iot.smartbulb.lightingservice": map[string]interface{}{
				"transition_light_state": state,
			},
		}
	case "turn_on":
		actionDescription = fmt.Sprintf("Attempting to turn ON bulb at %s (T:%dms)...", *bulbIP, *transition)
		state := map[string]interface{}{
			"on_off":         1,
			"ignore_default": 1,
		}
		if *transition > 0 {
			state["transition_period"] = *transition
		}
		commandPayload = map[string]interface{}{
			"smartlife.iot.smartbulb.lightingservice": map[string]interface{}{
				"transition_light_state": state,
			},
		}
	case "turn_off":
		actionDescription = fmt.Sprintf("Attempting to turn OFF bulb at %s (T:%dms)...", *bulbIP, *transition)
		state := map[string]interface{}{
			"on_off":         0,
			"ignore_default": 1,
		}
		if *transition > 0 {
			state["transition_period"] = *transition
		}
		commandPayload = map[string]interface{}{
			"smartlife.iot.smartbulb.lightingservice": map[string]interface{}{
				"transition_light_state": state,
			},
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
		state := map[string]interface{}{
			"on_off":         1, // Ensure bulb is on
			"ignore_default": 1,
			"brightness":     *val,
			// When setting brightness alone, we don't want to change color mode explicitly
			// So, we don't set hue, saturation, or color_temp unless they are already part of the bulb's current state concept for brightness adjustment.
			// For simplicity, we'll let the bulb decide how to interpret brightness-only changes.
		}
		if *transition > 0 {
			state["transition_period"] = *transition
		}
		commandPayload = map[string]interface{}{
			"smartlife.iot.smartbulb.lightingservice": map[string]interface{}{
				"transition_light_state": state,
			},
		}
	case "set_colortemp":
		if *kelvin == -1 {
			fmt.Println("Error: For set_colortemp, the -kelvin flag is required.")
			flag.Usage()
			os.Exit(1)
		}
		// Add Kelvin range validation if desired, e.g., 2500-9000
		if *kelvin < 2000 || *kelvin > 9000 { // Broad range, specific bulbs may vary
			fmt.Println("Error: Kelvin must be within a reasonable range (e.g., 2000-9000).")
			os.Exit(1)
		}
		valForTemp := 100 // Default to 100% brightness if -val not specified or use it if specified and valid
		if *val != -1 {
			if *val < 0 || *val > 100 {
				fmt.Println("Error: Value/Brightness for color temperature must be between 0 and 100.")
				os.Exit(1)
			}
			valForTemp = *val
		}
		actionDescription = fmt.Sprintf("Attempting to set color temperature to %dK at %d%% brightness (T:%dms) on bulb at %s...", *kelvin, valForTemp, *transition, *bulbIP)
		state := map[string]interface{}{
			"on_off":         1, // Ensure bulb is on
			"ignore_default": 1,
			"color_temp":     *kelvin,
			"brightness":     valForTemp,
			"hue":            0, // When setting color temp, hue/sat are not used
			"saturation":     0, // and should be set to 0 to indicate non-color mode.
		}
		if *transition > 0 {
			state["transition_period"] = *transition
		}
		commandPayload = map[string]interface{}{
			"smartlife.iot.smartbulb.lightingservice": map[string]interface{}{
				"transition_light_state": state,
			},
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

	fmt.Println(actionDescription)

	response, err := kasa.SendCommand(*bulbIP, commandPayload)
	if err != nil {
		fmt.Printf("Error sending command: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Successfully received response:")
	// For now, just print the map directly for simplicity
	for key, val := range response {
		fmt.Printf("%s: %v\n", key, val)
	}
}
