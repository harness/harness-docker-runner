package runtime

import (
	"bytes"
	"context"
	"testing"

	"github.com/harness/harness-docker-runner/api"
	"github.com/stretchr/testify/require"
)

func TestExecuteRunTestsV2StepReturnsErrorWhenCommandIsEmpty(t *testing.T) {
	req := &api.StartStepRequest{
		StartStepRequestConfig: api.StartStepRequestConfig{
			ID:   "test-step-id",
			Name: "test-step",
			Kind: api.RunTestsV2,
			RunTestsV2: api.RunTestsV2Config{
				Entrypoint: []string{"/bin/sh", "-c"},
			},
		},
	}

	_, _, _, _, _, _, err := executeRunTestsV2Step(context.Background(), nil, req, bytes.NewBuffer(nil), nil, nil)

	require.Error(t, err)
	require.Contains(t, err.Error(), "run tests v2 command cannot be empty")
}
