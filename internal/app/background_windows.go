package app

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var procSetProcessInformation = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetProcessInformation")

// lowerPriority puts this process below normal priority and asks Windows for EcoQoS (power
// throttling of execution speed): the scheduler favours the game and the window, and on
// laptops the work tends to run on efficiency cores at lower clocks. Best effort.
func lowerPriority() {
	_ = windows.SetPriorityClass(windows.CurrentProcess(), windows.BELOW_NORMAL_PRIORITY_CLASS)
	if procSetProcessInformation.Find() != nil {
		return // before Windows 8
	}
	const processPowerThrottling = 4 // PROCESS_INFORMATION_CLASS
	const executionSpeed = 1         // PROCESS_POWER_THROTTLING_EXECUTION_SPEED
	state := struct{ Version, ControlMask, StateMask uint32 }{1, executionSpeed, executionSpeed}
	procSetProcessInformation.Call(uintptr(windows.CurrentProcess()), processPowerThrottling, uintptr(unsafe.Pointer(&state)), unsafe.Sizeof(state))
}
