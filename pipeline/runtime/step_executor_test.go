package runtime

import (
	"fmt"
	"testing"

	"github.com/drone/runner-go/pipeline/runtime"
	"github.com/harness/harness-docker-runner/engine/spec"
	"github.com/stretchr/testify/assert"
)

func TestConvertStatus_NoErrorWhenStepSucceeds(t *testing.T) {
	status := StepStatus{
		Status:  Complete,
		State:   &runtime.State{Exited: true, ExitCode: 0},
		StepErr: nil,
	}
	resp := convertStatus(status)
	assert.Equal(t, 0, resp.ExitCode)
	assert.Empty(t, resp.Error)
}

func TestConvertStatus_ErrorPreservedWhenStepFails(t *testing.T) {
	status := StepStatus{
		Status:  Complete,
		State:   &runtime.State{Exited: true, ExitCode: 1},
		StepErr: fmt.Errorf("log upload failed"),
	}
	resp := convertStatus(status)
	assert.Equal(t, 255, resp.ExitCode)
	assert.Contains(t, resp.Error, "log upload failed")
}

func TestConvertStatus_ExitCode255WhenStepErrSetOnSuccess(t *testing.T) {
	status := StepStatus{
		Status:  Complete,
		State:   &runtime.State{Exited: true, ExitCode: 0},
		StepErr: fmt.Errorf("some error"),
	}
	resp := convertStatus(status)
	assert.Equal(t, 255, resp.ExitCode)
	assert.Contains(t, resp.Error, "some error")
}

func TestConvertStatus_NilState(t *testing.T) {
	status := StepStatus{
		Status:  Complete,
		State:   nil,
		StepErr: fmt.Errorf("log upload failed"),
	}
	resp := convertStatus(status)
	assert.Equal(t, 255, resp.ExitCode)
	assert.Contains(t, resp.Error, "log upload failed")
}

func TestCiLogServiceResilienceFFName(t *testing.T) {
	assert.Equal(t, "CI_LOG_SERVICE_RESILIENCE", ciLogServiceResilience)
}

func TestIsFeatureFlagEnabled_LogServiceResilience(t *testing.T) {
	tests := []struct {
		name string
		step *spec.Step
		want bool
	}{
		{"nil engine and nil step", nil, false},
		{"step with flag true", &spec.Step{Envs: map[string]string{ciLogServiceResilience: "true"}}, true},
		{"step with flag false", &spec.Step{Envs: map[string]string{ciLogServiceResilience: "false"}}, false},
		{"step with flag uppercase TRUE", &spec.Step{Envs: map[string]string{ciLogServiceResilience: "TRUE"}}, false},
		{"step without flag", &spec.Step{Envs: map[string]string{}}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsFeatureFlagEnabled(ciLogServiceResilience, nil, tc.step))
		})
	}
}
