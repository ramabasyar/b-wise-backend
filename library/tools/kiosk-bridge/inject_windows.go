//go:build windows

package main

import (
	"fmt"
	"log"
	"os/exec"
	"syscall"
	"time"
	"unsafe"
)

// KEYBDINPUT sesuai Win32; padding memastikan ukuran INPUT = 40 byte (amd64).
type kbInput struct {
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

type tagInput struct {
	typ uint32
	ki  kbInput
	_   [8]byte
}

const (
	inputKeyboard   = 1
	keyEventFKeyUp  = 0x0002
	errAlreadyExist = 183 // ERROR_ALREADY_EXISTS
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	procSendInput    = user32.NewProc("SendInput")
	procMessageBeep  = user32.NewProc("MessageBeep")
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procCreateMutexW = kernel32.NewProc("CreateMutexW")

	// mutexHandle disimpan agar mutex hidup sepanjang proses.
	mutexHandle uintptr
)

func init() {
	if unsafe.Sizeof(tagInput{}) != 40 {
		panic(fmt.Sprintf("ukuran INPUT tidak sesuai (%d byte) - build target salah", unsafe.Sizeof(tagInput{})))
	}
}

// singleInstance true bila ini satu-satunya instance (mutex baru dibuat).
func singleInstance() bool {
	h, _, err := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("Local\\bwise_library_visitor"))))
	mutexHandle = h
	if err == syscall.Errno(errAlreadyExist) {
		return false
	}
	return true
}

// openKiosk membuka URL di Edge kiosk fullscreen; fallback browser default.
func openKiosk(url string) {
	edge := exec.Command("cmd", "/c", "start", "", "msedge",
		"--kiosk", url, "--edge-kiosk-type=fullscreen", "--no-first-run")
	if err := edge.Run(); err == nil {
		log.Printf("browser kiosk dibuka (Edge fullscreen): %s", url)
		return
	}
	def := exec.Command("cmd", "/c", "start", "", url)
	if err := def.Run(); err != nil {
		log.Printf("gagal membuka browser otomatis (%v) - buka manual: %s", err, url)
		return
	}
	log.Printf("browser default dibuka: %s (Edge tidak ditemukan)", url)
}

func sendKey(vk uint16) error {
	down := tagInput{typ: inputKeyboard, ki: kbInput{wVk: vk}}
	up := tagInput{typ: inputKeyboard, ki: kbInput{wVk: vk, dwFlags: keyEventFKeyUp}}
	arr := []tagInput{down, up}
	ret, _, err := procSendInput.Call(uintptr(len(arr)), uintptr(unsafe.Pointer(&arr[0])), unsafe.Sizeof(arr[0]))
	if ret != uintptr(len(arr)) {
		return fmt.Errorf("SendInput gagal: %w", err)
	}
	return nil
}

// typeString mengetik string hex (0-9 A-F) sebagai virtual key, satu per satu.
func typeString(s string) error {
	for _, c := range s {
		switch {
		case (c >= '0' && c <= '9') || (c >= 'A' && c <= 'F') || (c >= 'a' && c <= 'f'):
		default:
			return fmt.Errorf("karakter tidak didukung untuk keyboard-wedge: %q", c)
		}
		vk := uint16(byte(c))
		if c >= 'a' && c <= 'f' {
			vk = uint16(byte(c - 32)) // uppercase
		}
		if err := sendKey(vk); err != nil {
			return err
		}
		time.Sleep(time.Duration(*flagDelay) * time.Millisecond)
	}
	return nil
}

func beepOK() {
	procMessageBeep.Call(0x0) // MB_OK
}
