package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// parsePorts expands port arguments such as "3000", ":3000", "8000-8010"
// and "3000,5173" into a sorted list of distinct ports.
func parsePorts(args []string) ([]int, error) {
	seen := map[int]bool{}
	var ports []int
	for _, arg := range args {
		for _, s := range strings.Split(arg, ",") {
			if s == "" {
				return nil, fmt.Errorf("invalid port list %q", arg)
			}
			lo, hi, err := parseRange(s)
			if err != nil {
				return nil, err
			}
			for p := lo; p <= hi; p++ {
				if !seen[p] {
					seen[p] = true
					ports = append(ports, p)
				}
			}
		}
	}
	sort.Ints(ports)
	return ports, nil
}

func parseRange(s string) (lo, hi int, err error) {
	a, b, isRange := strings.Cut(s, "-")
	if !isRange {
		p, err := parsePort(s)
		return p, p, err
	}
	if lo, err = parsePort(a); err != nil {
		return 0, 0, fmt.Errorf("invalid port range %q", s)
	}
	if hi, err = parsePort(b); err != nil {
		return 0, 0, fmt.Errorf("invalid port range %q", s)
	}
	if lo > hi {
		return 0, 0, fmt.Errorf("invalid port range %q: %d is greater than %d", s, lo, hi)
	}
	return lo, hi, nil
}

func parsePort(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimPrefix(s, ":"))
	if err != nil || n < 1 || n > 65535 {
		return 0, fmt.Errorf("invalid port %q", s)
	}
	return n, nil
}

// portSet reports whether a port was asked for.
type portSet map[int]bool

func newPortSet(ports []int) portSet {
	s := make(portSet, len(ports))
	for _, p := range ports {
		s[p] = true
	}
	return s
}

// describePorts formats ports compactly for messages: "port 3000",
// "ports 3000, 5173 and 8000-8010".
func describePorts(ports []int) string {
	var parts []string
	for i := 0; i < len(ports); {
		j := i
		for j+1 < len(ports) && ports[j+1] == ports[j]+1 {
			j++
		}
		if j-i >= 2 {
			parts = append(parts, fmt.Sprintf("%d-%d", ports[i], ports[j]))
		} else {
			for k := i; k <= j; k++ {
				parts = append(parts, strconv.Itoa(ports[k]))
			}
		}
		i = j + 1
	}
	switch {
	case len(parts) == 1 && len(ports) == 1:
		return "port " + parts[0]
	case len(parts) == 1:
		return "ports " + parts[0]
	}
	return "ports " + strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}
