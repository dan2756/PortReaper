package main

import (
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestParseArgs(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want options
	}{
		{[]string{"3000"}, options{ports: []int{3000}}},
		{[]string{"-y", "3000"}, options{ports: []int{3000}, yes: true}},
		{[]string{"3000", "--yes"}, options{ports: []int{3000}, yes: true}},
		{[]string{"-yu", "53"}, options{ports: []int{53}, yes: true, udp: true}},
		{[]string{"3000", "5173", "8000-8001"}, options{ports: []int{3000, 5173, 8000, 8001}}},
		{[]string{"--list"}, options{list: true}},
		{[]string{"-l", "--json", "--udp"}, options{list: true, json: true, udp: true}},
		{[]string{"--json", "3000"}, options{ports: []int{3000}, json: true}},
		{[]string{"--", "3000"}, options{ports: []int{3000}}},
		{[]string{"-9", "3000"}, options{ports: []int{3000}, signal: syscall.SIGKILL}},
		{[]string{"-s", "HUP", "3000"}, options{ports: []int{3000}, signal: syscall.SIGHUP}},
		{[]string{"--signal=INT", "3000"}, options{ports: []int{3000}, signal: syscall.SIGINT}},
		{[]string{"-yfw10s", "3000"}, options{ports: []int{3000}, yes: true, force: true, wait: 10 * time.Second}},
		{[]string{"--wait", "500ms", "--force", "3000"}, options{ports: []int{3000}, force: true, wait: 500 * time.Millisecond}},
		{[]string{"--help"}, options{help: true}},
		{[]string{"-V"}, options{version: true}},
	} {
		got, err := parseArgs(tc.args)
		if err != nil {
			t.Errorf("parseArgs(%q): %v", tc.args, err)
			continue
		}
		if tc.want.signal == 0 {
			tc.want.signal = syscall.SIGTERM
		}
		if tc.want.wait == 0 {
			tc.want.wait = 3 * time.Second
		}
		if !reflect.DeepEqual(*got, tc.want) {
			t.Errorf("parseArgs(%q) = %+v, want %+v", tc.args, *got, tc.want)
		}
	}
}

func TestParseArgsErrors(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, "no port given"},
		{[]string{"-y"}, "no port given"},
		{[]string{"--frobnicate", "3000"}, "unknown flag --frobnicate"},
		{[]string{"-x", "3000"}, "unknown flag -x"},
		{[]string{"--yes=1", "3000"}, "doesn't take a value"},
		{[]string{"3000", "--wait"}, "--wait needs a value"},
		{[]string{"3000", "-s"}, "-s needs a value"},
		{[]string{"--signal", "STOP", "3000"}, `unknown signal "STOP"`},
		{[]string{"-w", "soon", "3000"}, `invalid duration "soon"`},
		{[]string{"http"}, `invalid port "http"`},
		{[]string{"--", "-y"}, `"-y"`},
	} {
		_, err := parseArgs(tc.args)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("parseArgs(%q) error = %v, want it to mention %q", tc.args, err, tc.want)
		}
	}
}

func TestParseWait(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"3s": 3 * time.Second, "500ms": 500 * time.Millisecond, "1m": time.Minute,
		"5": 5 * time.Second, "0.5": 500 * time.Millisecond, "0": 0,
	} {
		got, err := parseWait(in)
		if err != nil || got != want {
			t.Errorf("parseWait(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "-1s", "-2", "soon", "3 s"} {
		if _, err := parseWait(in); err == nil {
			t.Errorf("parseWait(%q) should fail", in)
		}
	}
}
