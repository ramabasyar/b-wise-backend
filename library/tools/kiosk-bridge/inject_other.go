//go:build !windows

package main

import (
	"fmt"
	"time"
)

func singleInstance() bool { return true }

func openKiosk(url string) {
	fmt.Printf("[stub] openKiosk %s\n", url)
}

func sendKey(vk uint16) error {
	fmt.Printf("[stub] sendKey 0x%02X\n", vk)
	return nil
}

func typeString(s string) error {
	fmt.Printf("[stub] typeString %q delay=%dms\n", s, *flagDelay)
	time.Sleep(50 * time.Millisecond)
	return nil
}

func beepOK() {}
