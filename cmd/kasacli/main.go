package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/user/kasalightcontrol/kasa"
)

func main() {
	bulbIP := flag.String("ip", "", "IP address of the Kasa bulb")
	commandStr := flag.String("command", "get_sysinfo", "Command to execute: get_sysinfo or set_hsv")

	// Flags for set_hsv
	hue := flag.Int("hue", -1, "Hue (0-360) for set_hsv")
	sat := flag.Int("sat", -1, "Saturation (0-100) for set_hsv")
	val := flag.Int("val", -1, "Value/Brightness (0-100) for set_hsv")
	transition := flag.Int("transition", 0, "Transition period in milliseconds (optional)")

	flag.Parse()

	if *bulbIP == "" {
		fmt.Println("Error: Bulb IP address is required. Use the -ip flag.")
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
		hsvState := map[string]interface{}{
			"on_off":         1,
			"ignore_default": 1,
			"hue":            *hue,
			"saturation":     *sat,
			"brightness":     *val, // Kasa API uses 'brightness' for the V in HSV
			"color_temp":     0,    // Must be 0 for HSV mode
		}
		if *transition > 0 {
			hsvState["transition_period"] = *transition
		}
		commandPayload = map[string]interface{}{
			"smartlife.iot.smartbulb.lightingservice": map[string]interface{}{
				"transition_light_state": hsvState,
			},
		}
	default:
		fmt.Printf("Error: Unknown command '%s'. Valid commands are 'get_sysinfo' or 'set_hsv'.\n", *commandStr)
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
