package tools

import (
	"strconv"
	"testing"
)

func Test_isKubernetesEnvVar(t *testing.T) {
	tests := []struct {
		envKey string
		want   bool
	}{
		{"CENTRIFUGO_SERVICE_PORT_GRPC", true},
		{"CENTRIFUGO_PORT_8000_TCP", true},
	}
	for i, tt := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			if got := isKubernetesEnvVar(tt.envKey); got != tt.want {
				t.Errorf("isKubernetesEnvVar() = %v, want %v", got, tt.want)
			}
		})
	}
}
