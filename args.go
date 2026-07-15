package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const usage = `portkill - show and kill whatever is listening on a port

usage:
  portkill [flags] PORT...
  portkill --list [PORT...]

Shows the processes listening on the given TCP ports, then asks before
sending them SIGTERM and waits for the ports to be released. A PORT is a
number (3000), a range (8000-8010) or a comma-separated list of either.

flags:
  -y, --yes          don't ask for confirmation
  -s, --signal SIG   signal to send: TERM (default), INT, HUP, QUIT, KILL, USR1, USR2
  -9                 same as --signal KILL
  -w, --wait DUR     how long to wait for the ports to be free (default 3s; 0 = don't)
  -f, --force        send SIGKILL to processes still holding a port after --wait
  -n, --dry-run      show what would be killed, but don't
  -l, --list         list all listening ports instead of killing anything
      --json         print JSON; with ports, shows the listeners and exits
  -u, --udp          include UDP sockets
  -h, --help         show this help
  -V, --version      print the version

exit status:
  0  the processes were killed and the ports are free (always for --list)
  1  nothing is listening on the ports
  2  usage error, or listing sockets failed
  3  a kill failed, a port is still busy, or you answered no
`

type options struct {
	ports   []int // empty when listing everything
	yes     bool
	list    bool
	json    bool
	udp     bool
	signal  syscall.Signal
	wait    time.Duration
	force   bool
	dryRun  bool
	help    bool
	version bool
}

type flagSpec struct {
	short byte   // 0 for long-only flags
	long  string // "" for short-only flags
	value bool   // takes an argument
	set   func(o *options, v string) error
}

var flagSpecs = []flagSpec{
	{'y', "yes", false, func(o *options, _ string) error { o.yes = true; return nil }},
	{'s', "signal", true, func(o *options, v string) (err error) { o.signal, err = parseSignal(v); return err }},
	{'9', "", false, func(o *options, _ string) error { o.signal = syscall.SIGKILL; return nil }},
	{'w', "wait", true, func(o *options, v string) (err error) { o.wait, err = parseWait(v); return err }},
	{'f', "force", false, func(o *options, _ string) error { o.force = true; return nil }},
	{'n', "dry-run", false, func(o *options, _ string) error { o.dryRun = true; return nil }},
	{'l', "list", false, func(o *options, _ string) error { o.list = true; return nil }},
	{0, "json", false, func(o *options, _ string) error { o.json = true; return nil }},
	{'u', "udp", false, func(o *options, _ string) error { o.udp = true; return nil }},
	{'h', "help", false, func(o *options, _ string) error { o.help = true; return nil }},
	{'V', "version", false, func(o *options, _ string) error { o.version = true; return nil }},
}

func lookupLong(name string) *flagSpec {
	for i := range flagSpecs {
		if name != "" && flagSpecs[i].long == name {
			return &flagSpecs[i]
		}
	}
	return nil
}

func lookupShort(c byte) *flagSpec {
	for i := range flagSpecs {
		if c != 0 && flagSpecs[i].short == c {
			return &flagSpecs[i]
		}
	}
	return nil
}

// parseArgs parses GNU-style arguments: flags and ports may be mixed in any
// order, short flags may be combined (-yf), values may be attached (--wait=5s,
// -w5s) or separate, and "--" ends flag parsing.
func parseArgs(args []string) (*options, error) {
	o := &options{signal: syscall.SIGTERM, wait: 3 * time.Second}
	var positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			positional = append(positional, args[i+1:]...)
			i = len(args)
		case strings.HasPrefix(a, "--"):
			name, val, hasVal := strings.Cut(a[2:], "=")
			f := lookupLong(name)
			if f == nil {
				return nil, fmt.Errorf("unknown flag --%s", name)
			}
			if !f.value {
				if hasVal {
					return nil, fmt.Errorf("flag --%s doesn't take a value", name)
				}
			} else if !hasVal {
				if i+1 >= len(args) {
					return nil, fmt.Errorf("flag --%s needs a value", name)
				}
				i++
				val = args[i]
			}
			if err := f.set(o, val); err != nil {
				return nil, fmt.Errorf("--%s: %v", name, err)
			}
		case len(a) > 1 && a[0] == '-':
			for j := 1; j < len(a); j++ {
				f := lookupShort(a[j])
				if f == nil {
					return nil, fmt.Errorf("unknown flag -%c", a[j])
				}
				var val string
				if f.value {
					val = a[j+1:]
					if val == "" {
						if i+1 >= len(args) {
							return nil, fmt.Errorf("flag -%c needs a value", a[j])
						}
						i++
						val = args[i]
					}
					j = len(a)
				}
				if err := f.set(o, val); err != nil {
					return nil, fmt.Errorf("-%c: %v", f.short, err)
				}
			}
		default:
			positional = append(positional, a)
		}
	}
	if o.help || o.version {
		return o, nil
	}
	if len(positional) == 0 {
		if o.list {
			return o, nil
		}
		return nil, errors.New("no port given")
	}
	ports, err := parsePorts(positional)
	if err != nil {
		return nil, err
	}
	o.ports = ports
	return o, nil
}

// parseWait accepts a Go duration ("3s", "500ms") or a number of seconds.
func parseWait(s string) (time.Duration, error) {
	in := s
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		s += "s"
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("invalid duration %q", in)
	}
	return d, nil
}
