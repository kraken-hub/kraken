package util

import (
	"reflect"
	"testing"
)

func TestServiceProfiles(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want []string
	}{
		{name: "none", yaml: "svc:\n  image: nginx\n", want: nil},
		{name: "list", yaml: "svc:\n  profiles: [debug, mail]\n", want: []string{"debug", "mail"}},
		{name: "single string", yaml: "svc:\n  profiles: debug\n", want: []string{"debug"}},
		{name: "empty list", yaml: "svc:\n  profiles: []\n", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, ctx := mappingValue(t, tt.yaml, "svc")
			if got := ServiceProfiles(ctx, node); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestProfileActive(t *testing.T) {
	tests := []struct {
		name    string
		service []string
		active  []string
		want    bool
	}{
		{name: "no profiles always active", service: nil, active: nil, want: true},
		{name: "no profiles with other active", service: nil, active: []string{"debug"}, want: true},
		{name: "inactive", service: []string{"debug"}, active: nil, want: false},
		{name: "one matching", service: []string{"mail", "debug"}, active: []string{"debug"}, want: true},
		{name: "none matching", service: []string{"mail"}, active: []string{"debug"}, want: false},
		{name: "star", service: []string{"mail"}, active: []string{"*"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ProfileActive(tt.service, tt.active); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSplitList(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   []string
	}{
		{name: "nothing", values: nil, want: nil},
		{name: "empty string", values: []string{""}, want: nil},
		{name: "repeated and comma-separated", values: []string{"debug", "mail, extra", " ,ops"}, want: []string{"debug", "mail", "extra", "ops"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SplitList(tt.values...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}
