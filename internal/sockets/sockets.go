// Package sockets finds the processes that hold listening sockets.
//
// The data comes from a platform backend: /proc on Linux, lsof(8) elsewhere.
// Parsers are kept separate from the code that reads the live system so they
// can be tested against fixtures.
package sockets

import (
	"fmt"
	"os/user"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Listener is a listening socket together with the process that holds it.
// A socket shared by several processes (a forking server, say) shows up once
// per process. Name and Command are chosen by the process itself, so
// backends escape whatever in them doesn't print (see printable).
type Listener struct {
	Proto   string // "tcp", "tcp6", "udp" or "udp6"
	Addr    string // local address including the port: "127.0.0.1:3000", "[::]:3000", "*:3000"
	Port    int
	PID     int // 0 when the owning process isn't visible to us
	PPID    int // 0 when unknown
	UID     int // -1 when unknown
	User    string
	Name    string // short process name
	Command string // full command line where available, otherwise Name
}

// Options select which sockets a Backend reports.
type Options struct {
	// UDP adds unconnected UDP sockets bound to a port, the closest UDP
	// has to listening.
	UDP bool
}

// Backend lists the listening sockets on this machine.
type Backend interface {
	// Name identifies the backend in error messages.
	Name() string
	// Listeners returns every listening socket the backend can see.
	Listeners(Options) ([]Listener, error)
}

// userCache resolves numeric uids to user names, remembering the answers.
type userCache map[int]string

func (c userCache) name(uid int) string {
	if uid < 0 {
		return ""
	}
	if n, ok := c[uid]; ok {
		return n
	}
	n := strconv.Itoa(uid)
	if u, err := user.LookupId(n); err == nil {
		n = u.Username
	}
	c[uid] = n
	return n
}

// printable escapes control characters, invalid UTF-8 and other runes that
// don't print ("\n", "\x1b", "\u202e"), so a process can't add lines to
// portkill's output or send escape sequences to the terminal through its
// name or command line. ps and lsof do the same.
func printable(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && n == 1:
			fmt.Fprintf(&b, `\x%02x`, s[i])
		case !unicode.IsGraphic(r):
			q := strconv.QuoteRuneToGraphic(r)
			b.WriteString(q[1 : len(q)-1])
		default:
			b.WriteString(s[i : i+n])
		}
		i += n
	}
	return b.String()
}
