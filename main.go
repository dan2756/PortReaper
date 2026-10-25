// Command portkill shows and kills whatever is listening on a port.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/dan2756/portkill/internal/sockets"
	"golang.org/x/term"
)

// Exit codes.
const (
	exitOK       = 0 // ports are free, or --list / --json / --dry-run had something to show; --wait 0 does not check
	exitNotFound = 1 // nothing listening on the ports
	exitUsage    = 2 // bad arguments or the backend failed
	exitFailed   = 3 // a kill failed, a port is still busy, or the user said no
)

// version is set at build time with -ldflags "-X main.version=v1.2.3".
var version = ""

func versionString() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

func main() {
	a := &app{
		backend:     sockets.Default(),
		stdin:       os.Stdin,
		stdout:      os.Stdout,
		stderr:      os.Stderr,
		interactive: term.IsTerminal(int(os.Stdin.Fd())),
		kill:        syscall.Kill,
		poll:        100 * time.Millisecond,
		self:        os.Getpid(),
		euid:        os.Geteuid(),
	}
	if fd := int(os.Stdout.Fd()); term.IsTerminal(fd) {
		if w, _, err := term.GetSize(fd); err == nil {
			a.width = w
		}
	}
	os.Exit(a.run(os.Args[1:]))
}

type app struct {
	backend     sockets.Backend
	stdin       io.Reader
	stdout      io.Writer
	stderr      io.Writer
	interactive bool // stdin is a terminal we can prompt on
	width       int  // terminal width for tables; 0 when stdout isn't a terminal
	kill        func(pid int, sig syscall.Signal) error
	poll        time.Duration // how often to check whether a port was released
	self        int           // our own pid, which we never signal
	euid        int
}

func (a *app) errorf(format string, args ...any) {
	fmt.Fprintf(a.stderr, "portkill: "+format+"\n", args...)
}

func (a *app) warnf(format string, args ...any) {
	fmt.Fprintf(a.stderr, "portkill: warning: "+format+"\n", args...)
}

func (a *app) run(args []string) int {
	opts, err := parseArgs(args)
	if err != nil {
		a.errorf("%v", err)
		fmt.Fprintln(a.stderr, "usage: portkill [flags] PORT... (see portkill --help)")
		return exitUsage
	}
	if opts.help {
		fmt.Fprint(a.stdout, usage)
		return exitOK
	}
	if opts.version {
		fmt.Fprintln(a.stdout, "portkill", versionString())
		return exitOK
	}

	found, err := a.find(opts)
	if err != nil {
		a.errorf("%s: %v", a.backend.Name(), err)
		return exitUsage
	}
	if len(found) == 0 && len(opts.ports) > 0 {
		if opts.json {
			printJSON(a.stdout, found)
		}
		hint := ""
		if h, ok := a.backend.(interface{ HidesOtherUsers() bool }); ok && h.HidesOtherUsers() && a.euid != 0 {
			hint = " (other users' processes are only visible with sudo)"
		}
		a.errorf("nothing listening on %s%s", describePorts(opts.ports), hint)
		if opts.list {
			return exitOK
		}
		return exitNotFound
	}
	if opts.list || opts.json {
		return a.show(found, opts, exitOK)
	}
	printTable(a.stdout, found, a.width)

	targets := a.pickTargets(found)
	if len(targets) == 0 {
		a.errorf("no process to kill")
		return exitFailed
	}
	if opts.dryRun {
		for _, t := range targets {
			fmt.Fprintf(a.stdout, "would send %s to %d (%s)\n", signalName(opts.signal), t.PID, t.Name)
		}
		if opts.force && opts.signal != syscall.SIGKILL {
			fmt.Fprintf(a.stdout, "and SIGKILL to any that still hold a port after %v\n", opts.wait)
		}
		return exitOK
	}
	if !opts.yes {
		if !a.interactive {
			a.errorf("stdin is not a terminal; pass -y to kill without asking")
			return exitUsage
		}
		if !a.confirm(fmt.Sprintf("kill %s?", plural(len(targets), "process", "processes"))) {
			return exitFailed
		}
	}

	return a.killAll(found, targets, opts)
}

// find returns the listeners on the requested ports, or all of them when no
// ports were given, sorted by port.
func (a *app) find(opts *options) ([]sockets.Listener, error) {
	all, err := a.backend.Listeners(sockets.Options{UDP: opts.udp})
	if err != nil {
		return nil, err
	}
	all = sortListeners(all)
	if len(opts.ports) == 0 {
		return all, nil
	}
	want := newPortSet(opts.ports)
	var found []sockets.Listener
	for _, l := range all {
		if want[l.Port] {
			found = append(found, l)
		}
	}
	return found, nil
}

// portsOf returns the distinct ports of ls, which is sorted by port.
func portsOf(ls []sockets.Listener) []int {
	var ports []int
	for _, l := range ls {
		if n := len(ports); n == 0 || ports[n-1] != l.Port {
			ports = append(ports, l.Port)
		}
	}
	return ports
}

// isAre is describePorts with the right verb: "port 3000 is", "ports 1 and
// 2 are".
func isAre(ports []int) string {
	if len(ports) == 1 {
		return describePorts(ports) + " is"
	}
	return describePorts(ports) + " are"
}

// show prints listeners without killing anything.
func (a *app) show(ls []sockets.Listener, opts *options, code int) int {
	if opts.json {
		if err := printJSON(a.stdout, ls); err != nil {
			a.errorf("%v", err)
			return exitUsage
		}
		return code
	}
	if opts.list {
		printList(a.stdout, ls, a.width)
	} else {
		printTable(a.stdout, ls, a.width)
	}
	if a.euid != 0 {
		for _, l := range ls {
			if l.PID <= 0 {
				a.warnf("some sockets belong to other users' processes; run with sudo to see who owns them")
				break
			}
		}
	}
	return code
}

func (a *app) confirm(question string) bool {
	fmt.Fprintf(a.stderr, "%s [y/N] ", question)
	line, err := bufio.NewReader(a.stdin).ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(a.stderr)
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
