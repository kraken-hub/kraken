package config

import "testing"

func TestFormatContext(t *testing.T) {
	tests := []struct {
		name, env, profile, file, want string
	}{
		{name: "set", env: "dev,local", profile: "debug", file: "kraken.yaml", want: "env: dev,local\nprofile: debug\nsource: kraken.yaml\n"},
		{name: "empty profile hidden", env: "dev", profile: "", file: "kraken.yaml", want: "env: dev\nsource: kraken.yaml\n"},
		{name: "empty env and profile hidden", env: "", profile: "", file: "kraken.yaml", want: "source: kraken.yaml\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatContext(tt.env, tt.profile, tt.file); got != tt.want {
				t.Errorf("formatContext() = %q, want %q", got, tt.want)
			}
		})
	}
}
