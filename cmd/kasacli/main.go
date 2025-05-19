package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/user/kasalightcontrol/kasa"
)

func main() {
	bulbIP := flag.String("ip", "", "IP address of the Kasa bulb")
	flag.Parse()

	if *bulbIP == "" {
		fmt.Println("Error: Bulb IP address is required. Use the -ip flag.")
		flag.Usage()
		os.Exit(1)
	}

	fmt.Printf("Attempting to get system info from bulb at %s...\n", *bulbIP)

	// Construct the get_sysinfo command payload
	// For system commands, the top-level key is usually just "system".
	command := map[string]interface{}{
		"system": map[string]interface{}{
			"get_sysinfo": nil,
		},
	}

	response, err := kasa.SendCommand(*bulbIP, command)
	if err != nil {
		fmt.Printf("Error sending command: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Successfully received response:")
	// Pretty print the JSON response
	// jsonData, jsonErr := json.MarshalIndent(response, "", "  ")
	// if jsonErr != nil {
	// 	 fmt.Printf("Error formatting JSON response: %v\nRaw response: %v\n", jsonErr, response)
	// 	 return
	// }
	// fmt.Println(string(jsonData))

	// For now, just print the map directly for simplicity
	for key, val := range response {
		fmt.Printf("%s: %v\n", key, val)
	}
}
