// wl-pinch-probe opens a tiny Wayland window via libwayland-client and
// subscribes to wp_pointer_gestures_v1. Prints pinch begin/update/end events
// to stdout so we can verify whether the compositor delivers gestures.
package main

/*
#cgo pkg-config: wayland-client
#cgo CFLAGS: -Wall -Wno-unused-parameter
#include "wlprobe.h"
*/
import "C"

import "os"

func main() {
	os.Exit(int(C.wlprobe_run()))
}
