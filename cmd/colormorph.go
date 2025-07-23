package main

import (
	"fmt"
	"math"
	"math/rand"
	"os/exec"
	"time"
)

// Configurable section
var (
	interval     = 500 * time.Millisecond // Update interval
	steps        = 255                    // Number of steps for full morph
	bedroomIP    = "192.168.2.7"
	livingRoomIP = "192.168.2.8"
	colorA       = [3]int{255, 0, 128} // Living room start (magenta)
	colorB       = [3]int{0, 204, 204} // Bedroom start (cyan)
	brightness   = 80                  // Both bulbs, percent
)

func lerp(a, b int, t float64) int {
	return int(math.Round(float64(a) + (float64(b)-float64(a))*t))
}

func main() {
	rand.Seed(time.Now().UnixNano())
	for {
		// Forward morph: A->B and B->A
		for i := 0; i <= steps; i++ {
			t := float64(i) / float64(steps)

			// Occasionally (5% chance), instantly jump both bulbs to a random color
			if rand.Float64() < 0.05 {
				rJump := rand.Intn(256)
				gJump := rand.Intn(256)
				bJump := rand.Intn(256)
				go setBulbColor(livingRoomIP, rJump, gJump, bJump)
				go setBulbColor(bedroomIP, rJump, gJump, bJump)
				// Short pause after jump
				time.Sleep(time.Duration(1000+rand.Intn(2000)) * time.Millisecond)
				continue
			}

			// Living room goes A->B
			r1 := lerp(colorA[0], colorB[0], t)
			g1 := lerp(colorA[1], colorB[1], t)
			b1 := lerp(colorA[2], colorB[2], t)
			// Bedroom goes B->A
			r2 := lerp(colorB[0], colorA[0], t)
			g2 := lerp(colorB[1], colorA[1], t)
			b2 := lerp(colorB[2], colorA[2], t)

			go setBulbColor(livingRoomIP, r1, g1, b1)
			go setBulbColor(bedroomIP, r2, g2, b2)

			// Occasionally (5% chance), long pause (10–30s)
			if rand.Float64() < 0.05 {
				pause := time.Duration(10+rand.Intn(21)) * time.Second
				time.Sleep(pause)
			} else {
				// Random interval between 1s and 7s
				delay := time.Duration(1000+rand.Intn(6000)) * time.Millisecond
				time.Sleep(delay)
			}
		}
		// Reverse direction for continuous loop
		for i := steps - 1; i >= 0; i-- {
			t := float64(i) / float64(steps)

			if rand.Float64() < 0.05 {
				rJump := rand.Intn(256)
				gJump := rand.Intn(256)
				bJump := rand.Intn(256)
				go setBulbColor(livingRoomIP, rJump, gJump, bJump)
				go setBulbColor(bedroomIP, rJump, gJump, bJump)
				time.Sleep(time.Duration(1000+rand.Intn(2000)) * time.Millisecond)
				continue
			}

			r1 := lerp(colorA[0], colorB[0], t)
			g1 := lerp(colorA[1], colorB[1], t)
			b1 := lerp(colorA[2], colorB[2], t)
			r2 := lerp(colorB[0], colorA[0], t)
			g2 := lerp(colorB[1], colorA[1], t)
			b2 := lerp(colorB[2], colorA[2], t)

			go setBulbColor(livingRoomIP, r1, g1, b1)
			go setBulbColor(bedroomIP, r2, g2, b2)

			if rand.Float64() < 0.05 {
				pause := time.Duration(10+rand.Intn(21)) * time.Second
				time.Sleep(pause)
			} else {
				delay := time.Duration(1000+rand.Intn(6000)) * time.Millisecond
				time.Sleep(delay)
			}
		}
	}
}


func setBulbColor(ip string, r, g, b int) {
	cmd := exec.Command("/Users/jackmangano/Documents/testing/Kasa Light Apps/KasaLightControl/cmd/kasacli/kasacli",
		"-command", "set_hsv",
		"-ip", ip,
		"-r", fmt.Sprintf("%d", r),
		"-g", fmt.Sprintf("%d", g),
		"-b", fmt.Sprintf("%d", b),
		"-val", fmt.Sprintf("%d", brightness),
	)
	_ = cmd.Run() // Ignore errors for smooth morphing
}
