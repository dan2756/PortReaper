package main

import (
	"bytes"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/dan2756/portkill/internal/sockets"
)

// fakeSystem is a Backend plus a kill function: processes go away (and
// release their ports) when signalled, unless told otherwise.
type fakeSystem struct {
	mu        sync.Mutex
	listeners []sockets.Listener
	sent      []string     // "pid SIGNAME" in order
	stubborn  map[int]bool // ignore everything but SIGKILL
	denied    map[int]bool // kill fails with EPERM
}

func (f *fakeSystem) Name() string { return "fake" }

func (f *fakeSystem) Listeners(sockets.Options) ([]sockets.Listener, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sockets.Listener(nil), f.listeners...), nil
}

func (f *fakeSystem) kill(pid int, sig syscall.Signal) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.denied[pid] {
		return syscall.EPERM
	}
	alive := false
	for _, l := range f.listeners {
		alive = alive || l.PID == pid
	}
	if !alive {
		return syscall.ESRCH
	}
	f.sent = append(f.sent, strings.Join([]string{strconv.Itoa(pid), signalName(sig)}, " "))
	if sig != syscall.SIGKILL && f.stubborn[pid] {
		return nil
	}
	kept := f.listeners[:0]
	for _, l := range f.listeners {
		if l.PID != pid {
			kept = append(kept, l)
		}
	}
	f.listeners = kept
	return nil
}

type result struct {
	code           int
	stdout, stderr string
	sent           []string
}

func runFake(t *testing.T, f *fakeSystem, stdin string, interactive bool, args ...string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	a := &app{
		backend:     f,
		stdin:       strings.NewReader(stdin),
		stdout:      &stdout,
		stderr:      &stderr,
		interactive: interactive,
		kill:        f.kill,
		poll:        time.Millisecond,
	}
	code := a.run(args)
	return result{code, stdout.String(), stderr.String(), f.sent}
}

func node(pid, port int) sockets.Listener {
	return sockets.Listener{
		Proto: "tcp", Addr: "127.0.0.1:" + strconv.Itoa(port), Port: port,
		PID: pid, UID: 1000, User: "dj", Name: "node", Command: "node server.js",
	}
}

func (r result) expect(t *testing.T, code int, sent []string, stdout ...string) {
	t.Helper()
	if r.code != code {
		t.Errorf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", r.code, code, r.stdout, r.stderr)
	}
	if strings.Join(r.sent, ", ") != strings.Join(sent, ", ") {
		t.Errorf("signals sent = %v, want %v", r.sent, sent)
	}
	for _, s := range stdout {
		if !strings.Contains(r.stdout+r.stderr, s) {
			t.Errorf("output doesn't mention %q:\n%s%s", s, r.stdout, r.stderr)
		}
	}
}

func TestKillFreesPort(t *testing.T) {
	f := &fakeSystem{listeners: []sockets.Listener{node(100, 3000), node(200, 8080)}}
	runFake(t, f, "", false, "-y", "3000").
		expect(t, exitOK, []string{"100 SIGTERM"}, "sent SIGTERM to 100 (node)", "port 3000 is free")
}

func TestKillSeveralPorts(t *testing.T) {
	f := &fakeSystem{listeners: []sockets.Listener{node(100, 3000), node(200, 8001), node(200, 8002)}}
	r := runFake(t, f, "", false, "-y", "3000", "8000-8010")
	r.expect(t, exitOK, []string{"100 SIGTERM", "200 SIGTERM"},
		"port 3000 is free", "port 8001 is free", "port 8002 is free")
	if strings.Contains(r.stdout, "8000") {
		t.Errorf("ports that were already free shouldn't be reported:\n%s", r.stdout)
	}
}

func TestStillBusyWithoutForce(t *testing.T) {
	f := &fakeSystem{listeners: []sockets.Listener{node(100, 3000)}, stubborn: map[int]bool{100: true}}
	runFake(t, f, "", false, "-y", "--wait", "20ms", "3000").
		expect(t, exitFailed, []string{"100 SIGTERM"}, "port 3000 is still in use by 100 (node)")
}

func TestForceEscalatesToKill(t *testing.T) {
	f := &fakeSystem{listeners: []sockets.Listener{node(100, 3000)}, stubborn: map[int]bool{100: true}}
	runFake(t, f, "", false, "-yf", "-w", "20ms", "3000").
		expect(t, exitOK, []string{"100 SIGTERM", "100 SIGKILL"}, "still holds port 3000", "port 3000 is free")
}

func TestSignalFlags(t *testing.T) {
	f := &fakeSystem{listeners: []sockets.Listener{node(100, 3000)}}
	runFake(t, f, "", false, "-y9", "3000").expect(t, exitOK, []string{"100 SIGKILL"})

	f = &fakeSystem{listeners: []sockets.Listener{node(100, 3000)}}
	runFake(t, f, "", false, "-y", "--signal", "int", "3000").expect(t, exitOK, []string{"100 SIGINT"})
}

func TestNoWait(t *testing.T) {
	f := &fakeSystem{listeners: []sockets.Listener{node(100, 3000)}, stubborn: map[int]bool{100: true}}
	r := runFake(t, f, "", false, "-y", "--wait", "0", "3000")
	r.expect(t, exitOK, []string{"100 SIGTERM"})
	if strings.Contains(r.stdout, "port 3000") {
		t.Errorf("--wait 0 shouldn't check the port:\n%s", r.stdout)
	}
}

func TestPermissionDenied(t *testing.T) {
	l := node(100, 22)
	l.User, l.Name = "root", "sshd"
	f := &fakeSystem{listeners: []sockets.Listener{l}, denied: map[int]bool{100: true}}
	runFake(t, f, "", false, "-y", "22").
		expect(t, exitFailed, nil, "permission denied sending SIGTERM to 100 (sshd, owned by root); try again with sudo")
}

func TestNothingListening(t *testing.T) {
	f := &fakeSystem{listeners: []sockets.Listener{node(100, 3000)}}
	runFake(t, f, "", false, "-y", "4000", "5000-5002").
		expect(t, exitNotFound, nil, "nothing listening on ports 4000 and 5000-5002")
}

func TestRefusesWithoutTerminal(t *testing.T) {
	f := &fakeSystem{listeners: []sockets.Listener{node(100, 3000)}}
	runFake(t, f, "y\n", false, "3000").expect(t, exitUsage, nil, "pass -y")
}

func TestPrompt(t *testing.T) {
	for answer, want := range map[string]int{"y\n": exitOK, "YES\n": exitOK, "n\n": exitFailed, "\n": exitFailed, "": exitFailed} {
		f := &fakeSystem{listeners: []sockets.Listener{node(100, 3000), node(101, 3000)}}
		r := runFake(t, f, answer, true, "3000")
		if r.code != want {
			t.Errorf("answer %q: exit code %d, want %d", answer, r.code, want)
		}
		if !strings.Contains(r.stderr, "kill 2 processes? [y/N]") {
			t.Errorf("answer %q: no prompt in %q", answer, r.stderr)
		}
		if killed := len(r.sent) > 0; killed != (want == exitOK) {
			t.Errorf("answer %q: sent %v", answer, r.sent)
		}
	}
}

func TestParseSignal(t *testing.T) {
	for in, want := range map[string]syscall.Signal{
		"TERM": syscall.SIGTERM, "sigkill": syscall.SIGKILL, "Int": syscall.SIGINT,
		"HUP": syscall.SIGHUP, "9": syscall.SIGKILL, "15": syscall.SIGTERM,
	} {
		got, err := parseSignal(in)
		if err != nil || got != want {
			t.Errorf("parseSignal(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "SIG", "STOP", "99", "-9"} {
		if _, err := parseSignal(in); err == nil {
			t.Errorf("parseSignal(%q) should fail", in)
		}
	}
}
