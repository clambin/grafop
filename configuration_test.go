package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_grafanaConfig_client(t *testing.T) {
	tests := []struct {
		name    string
		config  grafanaConfig
		wantErr require.ErrorAssertionFunc
	}{
		{"http", grafanaConfig{URL: "http://grafana.example.com"}, require.NoError},
		{"https", grafanaConfig{URL: "https://grafana.example.com"}, require.NoError},
		{"no scheme", grafanaConfig{URL: "grafana.example.com"}, require.Error},
		{"invalid", grafanaConfig{URL: "\n"}, require.Error},
		{"empty", grafanaConfig{}, require.Error},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.config.client()
			tt.wantErr(t, err)
		})
	}
}
