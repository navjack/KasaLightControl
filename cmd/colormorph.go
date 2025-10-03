package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// Configurable section tuned for continuous rainbow motion.
var (
	targetFrameInterval  = 16 * time.Millisecond // Update rate ≈40 Hz
	rainbowCycleDuration = 3600 * time.Second    // Time for a full 360° hue sweep
	phaseOffsetAmplitude = 90.0                  // Max hue separation between bulbs (degrees)
	phaseOffsetPeriod    = 60 * time.Second      // How long it takes the offset to swing end to end
	baseLightness        = 25.0                  // Perceptual brightness (L* 0–100)
	baseChroma           = 120.0                 // Colourfulness in LCh (C* ≥0)
	logFrameEvery        = 40                    // Log roughly once per second
	brightness           = 100                   // Bulb brightness percentage

	lampshadeIP  = "192.168.2.6"
	livingRoomIP = "192.168.2.8"
	bedroomIP    = "192.168.2.7"
)

var (
	singleLightnessSwing  = 12.0             // +/- L* modulation for single-light mode
	singleChromaSwing     = 45.0             // +/- C* modulation for single-light mode
	singleHueJitter       = 24.0             // Degrees of hue wobble for single-light mode
	singlePulsePeriod     = 14 * time.Second // Period for lightness/chroma pulsing in single-light mode
	singleHueJitterPeriod = 20 * time.Second // Period for hue wobble when only one lamp is active
)

type bulbInfo struct {
	option string
	label  string
	name   string
	ip     string
}

var availableBulbs = []bulbInfo{
	{option: "1", label: "Living room", name: "living room", ip: livingRoomIP},
	{option: "2", label: "Lampshade", name: "lampshade", ip: lampshadeIP},
	{option: "3", label: "Bedroom", name: "bedroom", ip: bedroomIP},
}

func allBulbs() []bulbInfo {
	bulbs := make([]bulbInfo, len(availableBulbs))
	copy(bulbs, availableBulbs)
	return bulbs
}

func describeSelection(selected []bulbInfo) string {
	if len(selected) == len(availableBulbs) {
		return "all lights"
	}
	names := make([]string, len(selected))
	for i, bulb := range selected {
		names[i] = bulb.name
	}
	return strings.Join(names, ", ")
}

func promptLightSelection() []bulbInfo {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Select lights to animate:")
	fmt.Println("  [Enter] All lights (default)")
	for _, bulb := range availableBulbs {
		fmt.Printf("  %s) %s\n", bulb.option, bulb.label)
	}
	fmt.Print("> ")

	input, err := reader.ReadString('\n')
	if err != nil {
		logEvent("Input error, defaulting to all lights: %v", err)
		return allBulbs()
	}

	tokens := strings.Fields(input)
	if len(tokens) == 0 || (len(tokens) == 1 && tokens[0] == "0") {
		return allBulbs()
	}

	lookup := make(map[string]bulbInfo, len(availableBulbs))
	for _, bulb := range availableBulbs {
		lookup[bulb.option] = bulb
	}

	selected := make([]bulbInfo, 0, len(tokens))
	seen := make(map[string]bool, len(tokens))

	for _, token := range tokens {
		bulb, ok := lookup[token]
		if !ok {
			fmt.Printf("Unknown selection %q ignored.\n", token)
			continue
		}
		if seen[bulb.option] {
			continue
		}
		selected = append(selected, bulb)
		seen[bulb.option] = true
	}

	if len(selected) == 0 {
		fmt.Println("No valid selections entered; using all lights.")
		return allBulbs()
	}

	return selected
}

type rgbColor struct {
	R, G, B int
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	for {
		if err := runAnimation(ctx); err != nil {
			if errors.Is(err, context.Canceled) {
				logEvent("Received interrupt, exiting.")
				return
			}
			logEvent("Animation loop ended unexpectedly: %v", err)
		}

		select {
		case <-ctx.Done():
			logEvent("Context canceled; exiting.")
			return
		default:
		}

		time.Sleep(250 * time.Millisecond)
		logEvent("Restarting animation loop.")
	}
}

func runAnimation(ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("animation panic recovered: %v", r)
		}
	}()

	selectedBulbs := promptLightSelection()
	if len(selectedBulbs) == 0 {
		selectedBulbs = allBulbs()
	}
	selectionLabel := describeSelection(selectedBulbs)

	if targetFrameInterval <= 0 {
		targetFrameInterval = 25 * time.Millisecond
	}
	ticker := time.NewTicker(targetFrameInterval)
	defer ticker.Stop()

	start := time.Now()
	frame := 0

	logEvent("Starting rainbow morph (%s): cycle=%s offsetAmplitude=%.1f° offsetPeriod=%s", selectionLabel, rainbowCycleDuration, phaseOffsetAmplitude, phaseOffsetPeriod)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case now := <-ticker.C:
			elapsed := now.Sub(start)
			baseHue := wrapDegrees(elapsed.Seconds() * 360.0 / rainbowCycleDuration.Seconds())

			if len(selectedBulbs) == 1 {
				bulb := selectedBulbs[0]
				l, c, hue := singleLightLCH(elapsed, baseHue)
				color := labToRGB(lchToLab(l, c, hue))
				setBulbColor(bulb.ip, color)

				frame++
				if logFrameEvery > 0 && frame%logFrameEvery == 0 {
					logEvent("Frame %d: %s hue %.1f° L*%.1f C*%.1f RGB(%d,%d,%d)",
						frame, bulb.name, hue, l, c, color.R, color.G, color.B)
				}
				continue
			}

			offset := offsetDegrees(elapsed)
			frame++
			logEnabled := logFrameEvery > 0 && frame%logFrameEvery == 0
			var logParts []string

			for i, bulb := range selectedBulbs {
				rel := 0.0
				if len(selectedBulbs) > 1 {
					rel = 1 - 2*float64(i)/float64(len(selectedBulbs)-1)
				}
				hue := wrapDegrees(baseHue + rel*offset)
				color := labToRGB(lchToLab(baseLightness, baseChroma, hue))
				setBulbColor(bulb.ip, color)

				if logEnabled {
					logParts = append(logParts,
						fmt.Sprintf("%s hue %.1f° RGB(%d,%d,%d)",
							bulb.name, hue, color.R, color.G, color.B))
				}
			}

			if logEnabled {
				logEvent("Frame %d: %s", frame, strings.Join(logParts, " | "))
			}
		}
	}
}

func offsetDegrees(elapsed time.Duration) float64 {
	if phaseOffsetAmplitude == 0 || phaseOffsetPeriod <= 0 {
		return 0
	}
	angle := 2 * math.Pi * elapsed.Seconds() / phaseOffsetPeriod.Seconds()
	return phaseOffsetAmplitude * math.Sin(angle)
}

func singleLightLCH(elapsed time.Duration, baseHue float64) (lightness, chroma, hue float64) {
	pulsePhase := 0.0
	if singlePulsePeriod > 0 {
		pulsePhase = 2 * math.Pi * elapsed.Seconds() / singlePulsePeriod.Seconds()
	}

	huePhase := 0.0
	if singleHueJitterPeriod > 0 {
		huePhase = 2 * math.Pi * elapsed.Seconds() / singleHueJitterPeriod.Seconds()
	}

	lightness = clampFloat(baseLightness+singleLightnessSwing*math.Sin(pulsePhase), 0, 100)
	chroma = math.Max(0, baseChroma+singleChromaSwing*math.Sin(pulsePhase+math.Pi/2))
	hue = wrapDegrees(baseHue + singleHueJitter*math.Sin(huePhase))
	return
}

func wrapDegrees(h float64) float64 {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	return h
}

func lchToLab(l, c, h float64) labColor {
	rad := h * math.Pi / 180.0
	return labColor{
		L: l,
		A: c * math.Cos(rad),
		B: c * math.Sin(rad),
	}
}

type labColor struct {
	L, A, B float64
}

func labToRGB(l labColor) rgbColor {
	fy := (l.L + 16) / 116
	fx := fy + l.A/500
	fz := fy - l.B/200

	x := inverseFLab(fx) * 0.95047
	y := inverseFLab(fy) * 1.00000
	z := inverseFLab(fz) * 1.08883

	r := x*3.2404542 + y*-1.5371385 + z*-0.4985314
	g := x*-0.9692660 + y*1.8760108 + z*0.0415560
	b := x*0.0556434 + y*-0.2040259 + z*1.0572252

	r = linearToSRGB(clamp01(r))
	g = linearToSRGB(clamp01(g))
	b = linearToSRGB(clamp01(b))

	return rgbColor{
		R: clampToByte(r),
		G: clampToByte(g),
		B: clampToByte(b),
	}
}

func inverseFLab(t float64) float64 {
	const epsilon = 216.0 / 24389.0
	if t*t*t > epsilon {
		return t * t * t
	}
	const kappa = 24389.0 / 27.0
	return (116.0*t - 16.0) / kappa
}

func linearToSRGB(c float64) float64 {
	if c <= 0 {
		return 0
	}
	if c >= 1 {
		return 1
	}
	if c <= 0.0031308 {
		return 12.92 * c
	}
	return 1.055*math.Pow(c, 1.0/2.4) - 0.055
}

func clampFloat(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clampToByte(v float64) int {
	v = clamp01(v)
	return int(math.Round(v * 255))
}

func setBulbColor(ip string, color rgbColor) {
	cmd := exec.Command("/Volumes/4terrybi/coding/Kasa Light Apps/KasaLightControl/cmd/kasacli/kasacli",
		"-command", "set_hsv",
		"-ip", ip,
		"-r", fmt.Sprintf("%d", color.R),
		"-g", fmt.Sprintf("%d", color.G),
		"-b", fmt.Sprintf("%d", color.B),
		"-val", fmt.Sprintf("%d", brightness),
	)

	if err := cmd.Run(); err != nil {
		logEvent("Warning: failed to set bulb %s: %v", ip, err)
	}
}

func logEvent(format string, args ...interface{}) {
	timestamp := time.Now().Format(time.RFC3339)
	message := fmt.Sprintf(format, args...)
	fmt.Printf("[%s] %s\n", timestamp, message)
}

// Re-used Lab helpers
func srgbToLinear(c float64) float64 {
	c = clamp01(c)
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func rgbToLab(c rgbColor) labColor {
	r := srgbToLinear(float64(c.R) / 255.0)
	g := srgbToLinear(float64(c.G) / 255.0)
	b := srgbToLinear(float64(c.B) / 255.0)

	x := r*0.4124564 + g*0.3575761 + b*0.1804375
	y := r*0.2126729 + g*0.7151522 + b*0.0721750
	z := r*0.0193339 + g*0.1191920 + b*0.9503041

	x /= 0.95047
	y /= 1.00000
	z /= 1.08883

	fx := fLab(x)
	fy := fLab(y)
	fz := fLab(z)

	return labColor{
		L: 116*fy - 16,
		A: 500 * (fx - fy),
		B: 200 * (fy - fz),
	}
}

func fLab(t float64) float64 {
	const epsilon = 216.0 / 24389.0
	const kappa = 24389.0 / 27.0
	if t > epsilon {
		return math.Cbrt(t)
	}
	return (kappa*t + 16.0) / 116.0
}

// Included for completeness; currently unused but kept for experimentation.
func deltaE(a, b labColor) float64 {
	dL := a.L - b.L
	dA := a.A - b.A
	dB := a.B - b.B
	return math.Sqrt(dL*dL + dA*dA + dB*dB)
}
