//go:build !linux

package main

import "syscall"

// signalIfSame is the non-Linux path. There is no pidfd, and the lsof
// backend doesn't record a start time, so this is an ordinary kill.
func signalIfSame(pid int, _ uint64, sig syscall.Signal) error {
	return syscall.Kill(pid, sig)
}
