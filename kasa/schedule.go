package kasa

import (
	"encoding/json"
	"fmt"
)

// SLightOptions defines the light state for a schedule rule.
// Based on s_light={'on_off': 1, 'mode': 'customize_preset', 'hue': 0, 'saturation': 0, 'color_temp': 3638, 'brightness': 73}
type SLightOptions struct {
	OnOff int    `json:"on_off"`
	Mode  string `json:"mode,omitempty"` // e.g., "customize_preset"
	Hue   int    `json:"hue"`
	Saturation int    `json:"saturation"`
	ColorTemp  int    `json:"color_temp"`
	Brightness int    `json:"brightness"`
}

// ScheduleRule defines the structure for an add_rule or edit_rule command payload.
// Fields are based on python-kasa RuleModule.Rule and observed JSON payloads.
type ScheduleRule struct {
	ID         string         `json:"id,omitempty"`          // Only for edit_rule, omit for add_rule
	Name       string         `json:"name"`
	Enable     int            `json:"enable"`               // 1 for true, 0 for false
	Wday       [7]int         `json:"wday"`                 // Sun,Mon,Tue,Wed,Thu,Fri,Sat (e.g., [1,1,1,1,1,1,1])
	Repeat     int            `json:"repeat"`               // 1 for true, 0 for false (for daily/weekly schedules, this is true)
	SAct       int            `json:"sact"`                 // Start action: 1 for TurnOn, 0 for TurnOff. Potentially 2 if s_light dictates.
	STimeOpt   int            `json:"stime_opt"`            // Start time option: 0 for smin is absolute minutes from midnight.
	SMin       int            `json:"smin"`                 // Start time in minutes from midnight (0-1439).
	SLight     *SLightOptions `json:"s_light,omitempty"`     // Light state to set at start time.
	EAct       int            `json:"eact,omitempty"`       // End action, usually -1 or omitted for point-in-time rules.
	ETimeOpt   int            `json:"etime_opt,omitempty"`  // End time option, usually -1 or omitted.
	EMin       int            `json:"emin,omitempty"`       // End minutes, usually 0 or omitted.
}

// AddScheduleRule sends an 'add_rule' command to a Kasa device.
// For bulbs, the service is 'smartlife.iot.common.schedule'.
func AddScheduleRule(ip string, rule ScheduleRule) (json.RawMessage, error) {
	// Ensure ID is not set for add_rule, as the device assigns it.
	rule.ID = ""

	addRuleCmd := map[string]interface{}{
		"add_rule": rule,
	}

	payload := map[string]interface{}{
		"smartlife.iot.common.schedule": addRuleCmd,
	}

	// Call the exported SendCommand function from kasa.go
	responseMap, err := SendCommand(ip, payload) // SendCommand is in the same package
	if err != nil {
		return nil, fmt.Errorf("failed to send add_rule command to %s via SendCommand: %w", ip, err)
	}

	// The responseMap is map[string]interface{}. Convert it to json.RawMessage if that's the desired return type.
	// This maintains the previous function signature.
	rawResponse, err := json.Marshal(responseMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response from SendCommand for add_rule: %w", err)
	}

	// TODO: Parse the response to confirm success and extract new rule ID.
	// Example structure: {"smartlife.iot.common.schedule":{"add_rule":{"id":"NEW_RULE_ID","err_code":0}}}
	// For now, we return the raw marshaled response.
	return rawResponse, nil
}
