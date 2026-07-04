package main

import (
	"reflect"
	"testing"
)

func TestParsePorts(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want []int
	}{
		{[]string{"3000"}, []int{3000}},
		{[]string{":3000"}, []int{3000}},
		{[]string{"8000-8003"}, []int{8000, 8001, 8002, 8003}},
		{[]string{"5173", "3000"}, []int{3000, 5173}},
		{[]string{"3000,3001", "3000"}, []int{3000, 3001}},
		{[]string{"3000", "5173", "8000-8002"}, []int{3000, 5173, 8000, 8001, 8002}},
		{[]string{"9000-9000"}, []int{9000}},
		{[]string{"1", "65535"}, []int{1, 65535}},
		{[]string{"8001-8002", "8000-8001"}, []int{8000, 8001, 8002}},
	} {
		got, err := parsePorts(tc.args)
		if err != nil {
			t.Errorf("parsePorts(%q): %v", tc.args, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("parsePorts(%q) = %v, want %v", tc.args, got, tc.want)
		}
	}
}

func TestParsePortsErrors(t *testing.T) {
	for _, arg := range []string{
		"", "0", "65536", "-1", "abc", "30OO", "3000-", "-3000", "8010-8000",
		"1-2-3", "3000,", ",3000", "3000,,3001", "80-http", " 80",
	} {
		if got, err := parsePorts([]string{arg}); err == nil {
			t.Errorf("parsePorts(%q) = %v, want an error", arg, got)
		}
	}
}

func TestDescribePorts(t *testing.T) {
	for _, tc := range []struct {
		ports []int
		want  string
	}{
		{[]int{3000}, "port 3000"},
		{[]int{3000, 3001}, "ports 3000 and 3001"},
		{[]int{3000, 5173}, "ports 3000 and 5173"},
		{[]int{8000, 8001, 8002}, "ports 8000-8002"},
		{[]int{3000, 5173, 8000, 8001, 8002, 8003}, "ports 3000, 5173 and 8000-8003"},
		{[]int{22, 80, 443}, "ports 22, 80 and 443"},
	} {
		if got := describePorts(tc.ports); got != tc.want {
			t.Errorf("describePorts(%v) = %q, want %q", tc.ports, got, tc.want)
		}
	}
}
