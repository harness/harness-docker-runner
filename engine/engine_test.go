package engine

import (
	"testing"

	"github.com/harness/harness-docker-runner/engine/spec"
	"github.com/stretchr/testify/assert"
)

const logResilienceFF = "CI_LOG_SERVICE_RESILIENCE"

func TestIsFeatureFlagEnabled(t *testing.T) {
	tests := []struct {
		name           string
		pipelineConfig *spec.PipelineConfig
		want           bool
	}{
		{
			name:           "nil pipeline config",
			pipelineConfig: nil,
			want:           false,
		},
		{
			name:           "nil envs",
			pipelineConfig: &spec.PipelineConfig{},
			want:           false,
		},
		{
			name:           "empty envs",
			pipelineConfig: &spec.PipelineConfig{Envs: map[string]string{}},
			want:           false,
		},
		{
			name:           "flag absent",
			pipelineConfig: &spec.PipelineConfig{Envs: map[string]string{"OTHER_FF": "true"}},
			want:           false,
		},
		{
			name:           "flag true",
			pipelineConfig: &spec.PipelineConfig{Envs: map[string]string{logResilienceFF: "true"}},
			want:           true,
		},
		{
			name:           "flag false",
			pipelineConfig: &spec.PipelineConfig{Envs: map[string]string{logResilienceFF: "false"}},
			want:           false,
		},
		{
			name:           "flag uppercase TRUE is not accepted",
			pipelineConfig: &spec.PipelineConfig{Envs: map[string]string{logResilienceFF: "TRUE"}},
			want:           false,
		},
		{
			name:           "flag numeric 1 is not accepted",
			pipelineConfig: &spec.PipelineConfig{Envs: map[string]string{logResilienceFF: "1"}},
			want:           false,
		},
		{
			name:           "flag empty string",
			pipelineConfig: &spec.PipelineConfig{Envs: map[string]string{logResilienceFF: ""}},
			want:           false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := &Engine{pipelineConfig: tc.pipelineConfig}
			assert.Equal(t, tc.want, e.IsFeatureFlagEnabled(logResilienceFF))
		})
	}
}
