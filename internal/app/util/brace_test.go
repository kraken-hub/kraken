package util

import (
	"reflect"
	"testing"
)

func TestExpandBraces(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"no braces", "plain", []string{"plain"}},
		{"empty string", "", []string{""}},
		{"simple alternation", "{a,b,c}", []string{"a", "b", "c"}},
		{"alternation with prefix suffix", "host{1,2}.lh", []string{"host1.lh", "host2.lh"}},
		{"numeric range ascending", "{1..3}", []string{"1", "2", "3"}},
		{"numeric range descending", "{3..1}", []string{"3", "2", "1"}},
		{"numeric range single value", "{5..5}", []string{"5"}},
		{"numeric range with prefix suffix", "node{1..3}.app", []string{"node1.app", "node2.app", "node3.app"}},
		{"single item brace", "{onlyone}", []string{"onlyone"}},
		{"empty braces", "{}", []string{""}},
		{"non numeric range", "{a..c}", []string{"a..c"}},
		{"unmatched opening brace", "{abc", []string{"{abc"}},
		{"unmatched closing brace", "abc}", []string{"abc}"}},
		{"nested alternation", "{a,{b,c}}", []string{"a", "b", "c"}},
		{"combined alternation and range", "{a,{1..2}}", []string{"a", "1", "2"}},
		{"two separate brace groups", "{a,b}-{1,2}", []string{"a-1", "a-2", "b-1", "b-2"}},
		{"whitespace in range", "{1.. 3}", []string{"1", "2", "3"}},
		{"negative numeric range", "{-1..1}", []string{"-1", "0", "1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandBraces(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("expandBraces(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestBraceAlternatives(t *testing.T) {
	tests := []struct {
		name  string
		inner string
		want  []string
	}{
		{"comma list", "a,b,c", []string{"a", "b", "c"}},
		{"numeric range", "1..3", []string{"1", "2", "3"}},
		{"single non-range token", "solo", []string{"solo"}},
		{"nested brace inside alternative", "a,{b,c}", []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := braceAlternatives(tt.inner)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("braceAlternatives(%q) = %#v, want %#v", tt.inner, got, tt.want)
			}
		})
	}
}

func TestSplitTopLevelComma(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"no commas", "abc", []string{"abc"}},
		{"simple commas", "a,b,c", []string{"a", "b", "c"}},
		{"commas inside braces ignored", "a,{b,c},d", []string{"a", "{b,c}", "d"}},
		{"empty string", "", []string{""}},
		{"trailing comma", "a,", []string{"a", ""}},
		{"unbalanced closing brace ignored safely", "a,b}", []string{"a", "b}"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitTopLevelComma(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitTopLevelComma(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseNumericRange(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		wantOK bool
		wantLo int
		wantHi int
	}{
		{"valid range", "1..16", true, 1, 16},
		{"valid range with spaces", " 1 .. 16 ", true, 1, 16},
		{"no range separator", "abc", false, 0, 0},
		{"non-numeric bounds", "a..b", false, 0, 0},
		{"negative bound", "-3..3", true, -3, 3},
		{"reversed range", "5..1", true, 5, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			from, to, ok := parseNumericRange(tt.in)
			if ok != tt.wantOK {
				t.Fatalf("parseNumericRange(%q) ok = %v, want %v", tt.in, ok, tt.wantOK)
			}
			if ok && (from != tt.wantLo || to != tt.wantHi) {
				t.Errorf("parseNumericRange(%q) = (%d,%d), want (%d,%d)", tt.in, from, to, tt.wantLo, tt.wantHi)
			}
		})
	}
}

func TestNumericSequence(t *testing.T) {
	tests := []struct {
		name     string
		from, to int
		want     []string
	}{
		{"ascending", 1, 3, []string{"1", "2", "3"}},
		{"descending", 3, 1, []string{"3", "2", "1"}},
		{"single", 5, 5, []string{"5"}},
		{"crossing zero", -1, 1, []string{"-1", "0", "1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := numericSequence(tt.from, tt.to)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("numericSequence(%d,%d) = %#v, want %#v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}
