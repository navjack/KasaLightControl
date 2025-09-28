package main

import (
	"fmt"
	"math"
	"os/exec"
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
)

type rgbColor struct {
	R, G, B int
}

func main() {
	if targetFrameInterval <= 0 {
		targetFrameInterval = 25 * time.Millisecond
	}
	ticker := time.NewTicker(targetFrameInterval)
	defer ticker.Stop()

	start := time.Now()
	frame := 0

	logEvent("Starting rainbow morph: cycle=%s offsetAmplitude=%.1f° offsetPeriod=%s", rainbowCycleDuration, phaseOffsetAmplitude, phaseOffsetPeriod)

	for now := range ticker.C {
		elapsed := now.Sub(start)
		baseHue := wrapDegrees(elapsed.Seconds() * 360.0 / rainbowCycleDuration.Seconds())
		offset := offsetDegrees(elapsed)

		livingHue := wrapDegrees(baseHue + offset)
		lampHue := wrapDegrees(baseHue - offset)

		livingRGB := labToRGB(lchToLab(baseLightness, baseChroma, livingHue))
		lampRGB := labToRGB(lchToLab(baseLightness, baseChroma, lampHue))

		setBulbColor(livingRoomIP, livingRGB)
		setBulbColor(lampshadeIP, lampRGB)

		frame++
		if logFrameEvery > 0 && frame%logFrameEvery == 0 {
			logEvent("Frame %d: living hue %.1f° RGB(%d,%d,%d) | lampshade hue %.1f° RGB(%d,%d,%d)",
				frame, livingHue, livingRGB.R, livingRGB.G, livingRGB.B,
				lampHue, lampRGB.R, lampRGB.G, lampRGB.B)
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
