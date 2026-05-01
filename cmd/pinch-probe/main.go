// pinch-probe reads multi-touch events directly from /dev/input/event*
// and synthesizes pinch gestures from two-finger distance changes.
// Standalone validation of the evdev-based pinch path before integrating
// it into the main app.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"math"
	"os"

	"github.com/holoplot/go-evdev"
)

type slot struct {
	active bool
	x, y   int32
}

func main() {
	var (
		pathFlag = flag.String("path", "", "explicit /dev/input/eventN to read; default: auto-detect touchpad")
		listFlag = flag.Bool("list", false, "list candidate input devices and exit")
	)
	flag.Parse()

	if *listFlag {
		listDevices()
		return
	}

	dev, err := openTouchpad(*pathFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		if errors.Is(err, fs.ErrPermission) || errors.Is(err, os.ErrPermission) {
			fmt.Fprintln(os.Stderr, "hint: add yourself to the input group with `sudo usermod -aG input $USER` (then log out/in), or run with sudo for testing")
		}
		os.Exit(1)
	}
	defer dev.Close()

	name, _ := dev.Name()
	fmt.Printf("listening on %s — %q\n", dev.Path(), name)
	fmt.Println("place two fingers on the touchpad and pinch in/out; Ctrl+C to quit")

	const maxSlots = 16
	slots := make([]slot, maxSlots)
	current := 0

	var pinching bool
	var baseDist float64
	var lastReported float64

	for {
		ev, err := dev.ReadOne()
		if err != nil {
			fmt.Fprintln(os.Stderr, "read error:", err)
			return
		}

		switch ev.Type {
		case evdev.EV_ABS:
			switch ev.Code {
			case evdev.ABS_MT_SLOT:
				if v := int(ev.Value); v >= 0 && v < len(slots) {
					current = v
				}
			case evdev.ABS_MT_TRACKING_ID:
				slots[current].active = ev.Value != -1
			case evdev.ABS_MT_POSITION_X:
				slots[current].x = ev.Value
			case evdev.ABS_MT_POSITION_Y:
				slots[current].y = ev.Value
			}
		case evdev.EV_SYN:
			if ev.Code != evdev.SYN_REPORT {
				continue
			}
			active := activePoints(slots)
			switch {
			case len(active) == 2 && !pinching:
				baseDist = dist(active[0], active[1])
				if baseDist > 0 {
					pinching = true
					lastReported = 1.0
					fmt.Printf("pinch BEGIN  baseline=%.0f\n", baseDist)
				}
			case len(active) == 2 && pinching:
				d := dist(active[0], active[1])
				scale := d / baseDist
				if math.Abs(scale-lastReported) > 0.02 {
					fmt.Printf("pinch UPDATE scale=%.3f  d=%.0f\n", scale, d)
					lastReported = scale
				}
			case len(active) != 2 && pinching:
				fmt.Printf("pinch END    fingers=%d\n", len(active))
				pinching = false
			}
		}
	}
}

func listDevices() {
	paths, err := evdev.ListDevicePaths()
	if err != nil {
		fmt.Fprintln(os.Stderr, "list error:", err)
		os.Exit(1)
	}
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "no devices visible (likely a permissions issue — see hint above)")
		return
	}
	for _, p := range paths {
		d, err := evdev.Open(p.Path)
		if err != nil {
			fmt.Printf("%-22s  %-40s  (open failed: %v)\n", p.Path, p.Name, err)
			continue
		}
		mark := "        "
		if isTouchpad(d) {
			mark = "TOUCHPAD"
		}
		fmt.Printf("%s  %-22s  %s\n", mark, p.Path, p.Name)
		d.Close()
	}
}

func openTouchpad(explicit string) (*evdev.InputDevice, error) {
	if explicit != "" {
		return evdev.Open(explicit)
	}
	paths, err := evdev.ListDevicePaths()
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, errors.New("no input devices visible — check permissions on /dev/input/event*")
	}
	for _, p := range paths {
		d, err := evdev.Open(p.Path)
		if err != nil {
			continue
		}
		if isTouchpad(d) {
			return d, nil
		}
		d.Close()
	}
	return nil, errors.New("no touchpad found (looked for INPUT_PROP_POINTER + ABS_MT_POSITION_X)")
}

func isTouchpad(d *evdev.InputDevice) bool {
	hasPointer := false
	for _, p := range d.Properties() {
		switch p {
		case evdev.INPUT_PROP_DIRECT:
			return false // touchscreen, not touchpad
		case evdev.INPUT_PROP_POINTER:
			hasPointer = true
		}
	}
	if !hasPointer {
		return false
	}
	for _, code := range d.CapableEvents(evdev.EV_ABS) {
		if code == evdev.ABS_MT_POSITION_X {
			return true
		}
	}
	return false
}

func activePoints(s []slot) [][2]int32 {
	out := make([][2]int32, 0, 2)
	for _, sl := range s {
		if sl.active {
			out = append(out, [2]int32{sl.x, sl.y})
		}
	}
	return out
}

func dist(a, b [2]int32) float64 {
	dx := float64(a[0] - b[0])
	dy := float64(a[1] - b[1])
	return math.Sqrt(dx*dx + dy*dy)
}
