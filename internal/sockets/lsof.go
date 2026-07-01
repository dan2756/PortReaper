package sockets

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

// Lsof finds listening sockets by running lsof(8) and parsing its
// machine-readable field output (-F).
type Lsof struct {
	Path string // lsof binary; looked up in $PATH when empty
}

// Name implements Backend.
func (l *Lsof) Name() string { return "lsof" }

// lsofFields asks for pid, command, login, name, uid and parent pid.
const lsofFields = "pcLnuR"

// Listeners implements Backend.
func (l *Lsof) Listeners() ([]Listener, error) {
	out, err := l.run("-iTCP", "-sTCP:LISTEN")
	if err != nil {
		return nil, err
	}
	ls, err := parseLsof(bytes.NewReader(out), "tcp")
	if err != nil {
		return nil, err
	}
	addCommandLines(ls)
	return ls, nil
}

func (l *Lsof) run(selection ...string) ([]byte, error) {
	path := l.Path
	if path == "" {
		p, err := exec.LookPath("lsof")
		if err != nil {
			return nil, errors.New("lsof not found in $PATH")
		}
		path = p
	}
	args := append([]string{"-nP"}, selection...)
	args = append(args, "-F", lsofFields)
	cmd := exec.Command(path, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		// lsof exits 1 when nothing matched the selection.
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 && len(bytes.TrimSpace(out)) == 0 {
			return nil, nil
		}
		msg := strings.TrimSpace(stderr.String())
		if i := strings.IndexByte(msg, '\n'); i >= 0 {
			msg = msg[:i]
		}
		if msg != "" {
			return nil, fmt.Errorf("lsof: %v: %s", err, msg)
		}
		return nil, fmt.Errorf("lsof: %v", err)
	}
	return out, nil
}

// parseLsof parses lsof -F output. Each line starts with a field letter;
// a 'p' line opens a new process, and the process fields that follow apply
// to every file ('f'/'n' lines) up to the next 'p'.
func parseLsof(r io.Reader, proto string) ([]Listener, error) {
	var (
		out []Listener
		cur Listener
	)
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		val := line[1:]
		switch line[0] {
		case 'p':
			pid, err := strconv.Atoi(val)
			if err != nil || pid <= 0 {
				return nil, fmt.Errorf("lsof: bad pid %q", val)
			}
			cur = Listener{PID: pid, UID: -1}
		case 'R':
			cur.PPID, _ = strconv.Atoi(val)
		case 'c':
			cur.Name = val
		case 'L':
			cur.User = val
		case 'u':
			if uid, err := strconv.Atoi(val); err == nil {
				cur.UID = uid
			}
		case 'n':
			if cur.PID == 0 {
				return nil, fmt.Errorf("lsof: file %q before any process", val)
			}
			port, ok := lsofPort(val)
			if !ok {
				continue
			}
			l := cur
			l.Addr = val
			l.Port = port
			l.Proto = proto
			if strings.HasPrefix(val, "[") {
				l.Proto += "6"
			}
			if l.User == "" && l.UID >= 0 {
				l.User = strconv.Itoa(l.UID)
			}
			l.Command = l.Name
			out = append(out, l)
		}
	}
	return out, sc.Err()
}

// lsofPort returns the port of a local address such as "*:3000" or
// "[::1]:3000". Connected sockets ("a:1->b:2") and wildcard ports are
// rejected.
func lsofPort(name string) (int, bool) {
	if strings.Contains(name, "->") {
		return 0, false
	}
	i := strings.LastIndexByte(name, ':')
	if i < 0 {
		return 0, false
	}
	p, err := strconv.Atoi(name[i+1:])
	if err != nil || p <= 0 || p > 65535 {
		return 0, false
	}
	return p, true
}

// addCommandLines replaces the short lsof command names with full command
// lines from ps(1). It's best effort: on any failure the names stay.
func addCommandLines(ls []Listener) {
	seen := map[int]bool{}
	var pids []string
	for _, l := range ls {
		if !seen[l.PID] {
			seen[l.PID] = true
			pids = append(pids, strconv.Itoa(l.PID))
		}
	}
	if len(pids) == 0 {
		return
	}
	// ps exits non-zero if a pid vanished in the meantime; keep what it printed.
	out, _ := exec.Command("ps", "-ww", "-o", "pid=,command=", "-p", strings.Join(pids, ",")).Output()
	cmds := parsePs(out)
	for i := range ls {
		if c := cmds[ls[i].PID]; c != "" {
			ls[i].Command = c
		}
	}
}

// parsePs parses `ps -o pid=,command=` output into pid -> command line.
func parsePs(b []byte) map[int]string {
	cmds := map[int]string{}
	for _, line := range strings.Split(string(b), "\n") {
		pid, cmd, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(pid)
		if err != nil {
			continue
		}
		cmds[n] = strings.TrimSpace(cmd)
	}
	return cmds
}
