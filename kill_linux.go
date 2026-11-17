//go:build linux

package main

import (
	"errors"
	"os"
	"strconv"
	"syscall"

	"github.com/dan2756/portkill/internal/sockets"
	"golang.org/x/sys/unix"
)

// signalIfSame signals pid only when it is still the process that started
// at start. A pidfd pins that process so the pid can't be recycled between
// the check and the signal. Without pidfd, the start time is checked and
// then kill(2) is used. If the start time can't be re-read, the signal is
// sent anyway.
func signalIfSame(pid int, start uint64, sig syscall.Signal) error {
	fd, openErr := unix.PidfdOpen(pid, 0)
	if openErr == nil {
		defer unix.Close(fd)
	} else if errors.Is(openErr, syscall.ESRCH) {
		return openErr
	}

	if got, ok := procStart(pid); ok && got != start {
		return errPIDReused
	}
	if openErr != nil {
		return syscall.Kill(pid, sig)
	}
	return unix.PidfdSendSignal(fd, sig, nil, 0)
}

func procStart(pid int) (uint64, bool) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, false
	}
	return sockets.ParseProcStatStart(b)
}
