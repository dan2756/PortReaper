package main

import (
	"reflect"
	"strings"
	"testing"
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
		{[]string{"--help"}, options{help: true}},
		{[]string{"-V"}, options{version: true}},
	} {
		got, err := parseArgs(tc.args)
		if err != nil {
			t.Errorf("parseArgs(%q): %v", tc.args, err)
			continue
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
		{[]string{"http"}, `invalid port "http"`},
		{[]string{"--", "-y"}, `"-y"`},
	} {
		_, err := parseArgs(tc.args)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("parseArgs(%q) error = %v, want it to mention %q", tc.args, err, tc.want)
		}
	}
}
