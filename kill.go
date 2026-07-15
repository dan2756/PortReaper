package main

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
	"time"

	"github.com/dan2756/portkill/internal/sockets"
)

// killGrace is how long to wait for the port after escalating to SIGKILL,
// which the kernel acts on right away.
const killGrace = time.Second

var signals = []struct {
	name string
	sig  syscall.Signal
}{
	{"HUP", syscall.SIGHUP},
	{"INT", syscall.SIGINT},
	{"QUIT", syscall.SIGQUIT},
	{"KILL", syscall.SIGKILL},
	{"TERM", syscall.SIGTERM},
	{"USR1", syscall.SIGUSR1},
	{"USR2", syscall.SIGUSR2},
}

// parseSignal accepts "KILL", "SIGKILL", "kill" or "9".
func parseSignal(s string) (syscall.Signal, error) {
	name := strings.TrimPrefix(strings.ToUpper(s), "SIG")
	for _, e := range signals {
		if e.name == name || fmt.Sprint(int(e.sig)) == s {
			return e.sig, nil
		}
	}
	names := make([]string, len(signals))
	for i, e := range signals {
		names[i] = e.name
	}
	return 0, fmt.Errorf("unknown signal %q (want one of %s)", s, strings.Join(names, ", "))
}

func signalName(sig syscall.Signal) string {
	for _, e := range signals {
		if e.sig == sig {
			return "SIG" + e.name
		}
	}
	return fmt.Sprintf("signal %d", int(sig))
}

// uniqueProcs returns one entry per visible process, in order of appearance.
// Processes portkill can't see (pid 0) are left out.
func uniqueProcs(ls []sockets.Listener) []sockets.Listener {
	seen := map[int]bool{}
	var out []sockets.Listener
	for _, l := range ls {
		if l.PID <= 0 || seen[l.PID] {
			continue
		}
		seen[l.PID] = true
		out = append(out, l)
	}
	return out
}

// pickTargets returns the processes portkill may signal, one per pid, and
// explains the ones it won't touch: pid 1, itself, and sockets whose owner it
// can't see. It warns about processes that belong to someone else.
func (a *app) pickTargets(found []sockets.Listener) []sockets.Listener {
	var out []sockets.Listener
	for _, p := range uniqueProcs(found) {
		switch {
		case p.PID == 1:
			a.errorf("refusing to kill pid 1 (%s); if it holds port %d for a socket-activated service, stop that service instead",
				orDash(p.Name), p.Port)
			continue
		case p.PID == a.self:
			a.errorf("refusing to kill portkill itself (pid %d)", p.PID)
			continue
		}
		if a.euid != 0 && p.UID >= 0 && p.UID != a.euid {
			a.warnf("%d (%s) belongs to %s; you'll probably need sudo", p.PID, p.Name, orDash(p.User))
		}
		out = append(out, p)
	}

	var hidden []sockets.Listener
	for _, l := range found {
		if l.PID <= 0 {
			hidden = append(hidden, l)
		}
	}
	if ports := portsOf(hidden); len(ports) > 0 {
		owner := "another user"
		if u := hidden[0].User; u != "" {
			owner = u
		}
		if a.euid != 0 {
			a.warnf("%s held by a process of %s that only root can see; try again with sudo", isAre(ports), owner)
		} else {
			a.warnf("%s held by a process portkill can't see (owned by %s); it may be in another pid namespace", isAre(ports), owner)
		}
	}
	return out
}

// killAll signals the targets, waits for the ports in found to be released
// and reports the outcome.
func (a *app) killAll(found, targets []sockets.Listener, opts *options) int {
	ok, delivered := true, 0
	for _, t := range targets {
		if a.send(t, opts.signal) {
			delivered++
		} else {
			ok = false
		}
	}
	if delivered == 0 {
		return exitFailed // nothing to wait for
	}
	if opts.wait <= 0 && !opts.force {
		if !ok {
			return exitFailed
		}
		return exitOK
	}

	// Only watch the ports that were busy; something new appearing on
	// another port of a range isn't our business.
	watch := *opts
	watch.ports = portsOf(found)
	busy, err := a.waitFree(&watch, opts.wait)
	if err != nil {
		a.errorf("%s: %v", a.backend.Name(), err)
		return exitFailed
	}
	if len(busy) > 0 && opts.force && opts.signal != syscall.SIGKILL {
		signalled := map[int]bool{}
		for _, t := range targets {
			signalled[t.PID] = true
		}
		escalated := false
		for _, l := range uniqueProcs(busy) {
			if !signalled[l.PID] {
				continue
			}
			fmt.Fprintf(a.stdout, "%d (%s) still holds port %d after %v\n", l.PID, l.Name, l.Port, opts.wait)
			ok = a.send(l, syscall.SIGKILL) && ok
			escalated = true
		}
		if escalated {
			if busy, err = a.waitFree(&watch, killGrace); err != nil {
				a.errorf("%s: %v", a.backend.Name(), err)
				return exitFailed
			}
		}
	}

	if !a.report(watch.ports, busy, targets) {
		ok = false
	}
	if !ok {
		return exitFailed
	}
	return exitOK
}

// send delivers sig to t and says what happened. It returns false when the
// signal couldn't be delivered.
func (a *app) send(t sockets.Listener, sig syscall.Signal) bool {
	err := a.kill(t.PID, sig)
	switch {
	case err == nil:
		fmt.Fprintf(a.stdout, "sent %s to %d (%s)\n", signalName(sig), t.PID, t.Name)
		return true
	case errors.Is(err, syscall.ESRCH):
		fmt.Fprintf(a.stdout, "%d (%s) already exited\n", t.PID, t.Name)
		return true
	case errors.Is(err, syscall.EPERM):
		a.errorf("permission denied sending %s to %d (%s, owned by %s); try again with sudo",
			signalName(sig), t.PID, t.Name, orDash(t.User))
	default:
		a.errorf("sending %s to %d (%s): %v", signalName(sig), t.PID, t.Name, err)
	}
	return false
}

// waitFree polls until nothing listens on opts.ports or d has passed, and
// returns whatever is still listening.
func (a *app) waitFree(opts *options, d time.Duration) ([]sockets.Listener, error) {
	deadline := time.Now().Add(d)
	for {
		busy, err := a.find(opts)
		if err != nil || len(busy) == 0 || !time.Now().Before(deadline) {
			return busy, err
		}
		time.Sleep(min(a.poll, time.Until(deadline)))
	}
}

// report prints one line per port and returns false if any is still busy.
func (a *app) report(ports []int, busy, targets []sockets.Listener) bool {
	signalled := map[int]bool{}
	for _, t := range targets {
		signalled[t.PID] = true
	}
	holders := map[int][]sockets.Listener{}
	for _, l := range uniqueByPortPID(busy) {
		holders[l.Port] = append(holders[l.Port], l)
	}
	free := true
	for _, p := range ports {
		hs := holders[p]
		if len(hs) == 0 {
			fmt.Fprintf(a.stdout, "port %d is free\n", p)
			continue
		}
		free = false
		var who []string
		for _, h := range hs {
			switch {
			case h.PID <= 0:
				who = append(who, "a process we can't see")
			case signalled[h.PID]:
				who = append(who, fmt.Sprintf("%d (%s)", h.PID, h.Name))
			default:
				who = append(who, fmt.Sprintf("%d (%s, a new process; is something restarting it?)", h.PID, h.Name))
			}
		}
		a.errorf("port %d is still in use by %s", p, strings.Join(who, ", "))
	}
	return free
}

// uniqueByPortPID drops listeners that repeat a (port, pid) pair, so a
// process on both 0.0.0.0:22 and [::]:22 is mentioned once.
func uniqueByPortPID(ls []sockets.Listener) []sockets.Listener {
	type key struct{ port, pid int }
	seen := map[key]bool{}
	var out []sockets.Listener
	for _, l := range ls {
		k := key{l.Port, l.PID}
		if !seen[k] {
			seen[k] = true
			out = append(out, l)
		}
	}
	return out
}
