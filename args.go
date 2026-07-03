package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const usage = `portkill - show and kill whatever is listening on a port

usage:
  portkill [flags] PORT
  portkill --list [PORT]

Shows the processes listening on TCP port PORT, then asks before sending
them SIGTERM.

flags:
  -y, --yes       don't ask for confirmation
  -l, --list      list all listening ports instead of killing anything
      --json      print JSON; with a PORT, shows the listeners and exits
  -h, --help      show this help
  -V, --version   print the version
`

type options struct {
	port    int // 0 when listing everything
	yes     bool
	list    bool
	json    bool
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
	{'l', "list", false, func(o *options, _ string) error { o.list = true; return nil }},
	{0, "json", false, func(o *options, _ string) error { o.json = true; return nil }},
	{'h', "help", false, func(o *options, _ string) error { o.help = true; return nil }},
	{'V', "version", false, func(o *options, _ string) error { o.version = true; return nil }},
}

func lookupLong(name string) *flagSpec {
	for i := range flagSpecs {
		if flagSpecs[i].long == name {
			return &flagSpecs[i]
		}
	}
	return nil
}

func lookupShort(c byte) *flagSpec {
	for i := range flagSpecs {
		if flagSpecs[i].short == c {
			return &flagSpecs[i]
		}
	}
	return nil
}

// parseArgs parses GNU-style arguments: flags and ports may be mixed in any
// order, short flags may be combined (-yf), values may be attached (--wait=5s,
// -w5s) or separate, and "--" ends flag parsing.
func parseArgs(args []string) (*options, error) {
	o := &options{}
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
	switch len(positional) {
	case 0:
		if o.list {
			return o, nil
		}
		return nil, errors.New("no port given")
	case 1:
	default:
		return nil, errors.New("only one port at a time")
	}
	p, err := parsePort(positional[0])
	if err != nil {
		return nil, err
	}
	o.port = p
	return o, nil
}

func parsePort(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimPrefix(s, ":"))
	if err != nil || n < 1 || n > 65535 {
		return 0, fmt.Errorf("invalid port %q", s)
	}
	return n, nil
}
