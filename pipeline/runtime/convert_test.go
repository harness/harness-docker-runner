package runtime

import (
	"testing"

	"github.com/harness/harness-docker-runner/api"
	"github.com/stretchr/testify/assert"
)

func TestToStepInitializesMissingEnvs(t *testing.T) {
	step := toStep(&api.StartStepRequest{})
	assert.NotNil(t, step.Envs)
	assert.NotPanics(t, func() { step.Envs["DRONE_OUTPUT"] = "out" })
}

func TestToStepKeepsEnvs(t *testing.T) {
	envs := map[string]string{"A": "B"}
	step := toStep(&api.StartStepRequest{StartStepRequestConfig: api.StartStepRequestConfig{Envs: envs}})
	assert.Equal(t, envs, step.Envs)
}
