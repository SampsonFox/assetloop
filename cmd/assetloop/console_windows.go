package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Explorer gives a console program its own console; an existing shell shares it.
// A process with redirected output or without a console must never wait for input.
func ownsConsole() bool {
	var mode uint32
	input, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	if err != nil || windows.GetConsoleMode(input, &mode) != nil {
		return false
	}
	output, err := windows.GetStdHandle(windows.STD_ERROR_HANDLE)
	if err != nil || windows.GetConsoleMode(output, &mode) != nil {
		return false
	}
	var processes [2]uint32
	count, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleProcessList").Call(uintptr(unsafe.Pointer(&processes[0])), uintptr(len(processes)))
	return count == 1
}
