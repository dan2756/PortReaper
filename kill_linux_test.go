//go:build linux

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/dan2756/portkill/internal/sockets"
)

// exited reports whether pid is gone or a zombie. kill(pid, 0) still
// succeeds for a zombie, so it can't tell a delivered SIGTERM from a miss.
func exited(pid int) bool {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return true
	}
	r := bytes.LastIndexByte(b, ')')
	if r < 0 || r+2 >= len(b) {
		return false
	}
	fields := bytes.Fields(b[r+2:])
	return len(fields) > 0 && fields[0][0] == 'Z'
}

func TestDoesNotSignalReusedPID(t *testing.T) {
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	start, ok := procStart(pid)
	if !ok {
		t.Fatal("sleep has no start time")
	}

	var out bytes.Buffer
	a := &app{
		stdout: &out,
		stderr: &out,
		kill: func(int, syscall.Signal) error {
			t.Fatal("plain kill used for a process whose start time is known")
			return nil
		},
	}
	listed := sockets.Listener{PID: pid, Name: "sleep", HasStart: true, StartTicks: start + 1}
	if !a.send(listed, syscall.SIGTERM) {
		t.Fatal("send")
	}
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("pid %d was signalled despite a mismatched start time: %v", pid, err)
	}
	if !strings.Contains(out.String(), "different process") {
		t.Fatalf("output = %q", out.String())
	}

	out.Reset()
	listed.StartTicks = start
	if !a.send(listed, syscall.SIGTERM) {
		t.Fatal("send SIGTERM")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !exited(pid) {
		time.Sleep(10 * time.Millisecond)
	}
	if !exited(pid) {
		t.Fatal("process still running after SIGTERM to the matching start time")
	}
	if !strings.Contains(out.String(), "sent SIGTERM to") {
		t.Fatalf("output = %q", out.String())
	}
}
