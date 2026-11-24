//go:build cgo

package gui

import (
	"os"
	"sync"
	"syscall"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Signal handlers and Go's stacks (16 §8.1 "Signal handlers"). Go runs every signal handler on
// a separate signal stack; a handler installed without SA_ONSTACK runs on whatever stack the
// interrupted thread was using, which in a Go thread is a goroutine's stack of a few KB. A
// signal frame (about 3 KB with AVX-512 state) plus the handler can run past its end into
// another goroutine's stack or the heap, and the Go runtime later fails with "unknown caller
// pc" or "found pointer to free object".
//
// GLib before 2.74 (Ubuntu 22.04 has 2.72) installs such a SIGCHLD handler when WebKitGTK
// starts its first helper process (a child watch). Any child exiting afterwards (a scan or
// analysis subprocess, git, a test's node) then interrupts a random thread. Wails adds
// SA_ONSTACK to the common handlers after the web view is made and when a page finishes
// loading, which leaves the time in between; the GUI tests, which start GTK without Wails,
// had no repair at all. So before GTK starts, holdChildWatch makes GLib install its SIGCHLD
// handler once, for the whole process, and onSignalStack moves it to the signal stack;
// onSignalStack runs again once the window is up.
//
// SIGUSR1 is left alone: JavaScriptCore suspends threads for its garbage collector with it
// and must run on the suspended thread's own stack (Wails issue #5527: with SA_ONSTACK,
// WebKit stops).

// kernelSigaction is the kernel's struct sigaction on amd64 and arm64.
type kernelSigaction struct {
	handler  uintptr
	flags    uint64
	restorer uintptr
	mask     uint64
}

const saOnStack = 0x08000000

func sigaction(sig int, act, old *kernelSigaction) error {
	_, _, e := syscall.RawSyscall6(syscall.SYS_RT_SIGACTION, uintptr(sig),
		uintptr(unsafe.Pointer(act)), uintptr(unsafe.Pointer(old)), unsafe.Sizeof(act.mask), 0, 0)
	if e != 0 {
		return e
	}
	return nil
}

// offStack lists the signals whose handler would run on the interrupted stack, SIGUSR1 aside.
func offStack() []int {
	var sigs []int
	for sig := 1; sig < 65; sig++ {
		var a kernelSigaction
		if sig == int(syscall.SIGUSR1) || sig == int(syscall.SIGKILL) || sig == int(syscall.SIGSTOP) {
			continue
		}
		if sigaction(sig, nil, &a) != nil || a.handler <= 1 { // SIG_DFL, SIG_IGN
			continue
		}
		if a.flags&saOnStack == 0 {
			sigs = append(sigs, sig)
		}
	}
	return sigs
}

// onSignalStack adds SA_ONSTACK to every handler offStack lists and returns those signals.
func onSignalStack() []int {
	var fixed []int
	for _, sig := range offStack() {
		var a kernelSigaction
		if sigaction(sig, nil, &a) != nil {
			continue
		}
		a.flags |= saOnStack
		if sigaction(sig, &a, nil) == nil {
			fixed = append(fixed, sig)
		}
	}
	return fixed
}

var childWatch struct {
	once   sync.Once
	source uintptr
}

// holdChildWatch makes GLib install its SIGCHLD handler now, before any web view exists.
// GLib counts its child watches and installs the handler when the count leaves zero (and
// puts SIG_DFL back when it returns there), so one watch held for the life of the process
// makes it install the handler once and never again, and onSignalStack's repair stays.
// The watch is on a child that has already exited (true, waited for without reaping it):
// GLib reaps it as it makes the source, which is never attached and never freed. A watch on
// a process that is not a child would print a GLib warning at the first SIGCHLD. GLib 2.74
// and later watch children with pidfds and install no handler, so nothing is done there.
// It reports whether it holds a watch.
func holdChildWatch() bool {
	childWatch.once.Do(func() {
		var checkVersion func(major, minor, micro uint32) uintptr
		var newSource func(pid int32) uintptr
		for fn, name := range map[any]string{&checkVersion: "glib_check_version", &newSource: "g_child_watch_source_new"} {
			sym, err := purego.Dlsym(purego.RTLD_DEFAULT, name)
			if err != nil {
				return
			}
			purego.RegisterFunc(fn, sym)
		}
		if checkVersion(2, 74, 0) == 0 { // NULL: 2.74 or later
			return
		}
		p, err := os.StartProcess("/bin/true", []string{"true"}, &os.ProcAttr{})
		if err != nil {
			return
		}
		defer p.Release()
		const pPID, wExited, wNoWait = 1, 4, 0x01000000
		var info [128]byte // siginfo_t
		for {
			_, _, e := syscall.Syscall6(syscall.SYS_WAITID, pPID, uintptr(p.Pid), uintptr(unsafe.Pointer(&info)), wExited|wNoWait, 0, 0)
			if e != syscall.EINTR {
				if e != 0 {
					return
				}
				break
			}
		}
		childWatch.source = newSource(int32(p.Pid))
	})
	return childWatch.source != 0
}
