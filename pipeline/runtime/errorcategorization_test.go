package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/harness/harness-docker-runner/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsErrorCategorizationEnabled(t *testing.T) {
	tests := []struct {
		name     string
		envs     map[string]string
		expected bool
	}{
		{
			name:     "enabled with true",
			envs:     map[string]string{"CI_CUSTOM_ERROR_CATEGORIZATION": "true"},
			expected: true,
		},
		{
			name:     "enabled with True (case insensitive)",
			envs:     map[string]string{"CI_CUSTOM_ERROR_CATEGORIZATION": "True"},
			expected: true,
		},
		{
			name:     "enabled with TRUE (all caps)",
			envs:     map[string]string{"CI_CUSTOM_ERROR_CATEGORIZATION": "TRUE"},
			expected: true,
		},
		{
			name:     "disabled with false",
			envs:     map[string]string{"CI_CUSTOM_ERROR_CATEGORIZATION": "false"},
			expected: false,
		},
		{
			name:     "disabled when key missing",
			envs:     map[string]string{},
			expected: false,
		},
		{
			name:     "disabled with empty value",
			envs:     map[string]string{"CI_CUSTOM_ERROR_CATEGORIZATION": ""},
			expected: false,
		},
		{
			name:     "disabled with nil map",
			envs:     nil,
			expected: false,
		},
		{
			name:     "disabled with random value",
			envs:     map[string]string{"CI_CUSTOM_ERROR_CATEGORIZATION": "yes"},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isErrorCategorizationEnabled(tc.envs)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestShouldCategorizeError(t *testing.T) {
	tests := []struct {
		name     string
		exitCode int
		stepErr  error
		envs     map[string]string
		expected bool
	}{
		{
			name:     "step failed with non-zero exit and FF enabled",
			exitCode: 1,
			stepErr:  nil,
			envs:     map[string]string{"CI_CUSTOM_ERROR_CATEGORIZATION": "true"},
			expected: true,
		},
		{
			name:     "step failed with error and FF enabled",
			exitCode: 0,
			stepErr:  fmt.Errorf("some error"),
			envs:     map[string]string{"CI_CUSTOM_ERROR_CATEGORIZATION": "true"},
			expected: true,
		},
		{
			name:     "step failed with both error and non-zero exit",
			exitCode: 127,
			stepErr:  fmt.Errorf("command not found"),
			envs:     map[string]string{"CI_CUSTOM_ERROR_CATEGORIZATION": "true"},
			expected: true,
		},
		{
			name:     "step succeeded - no categorization",
			exitCode: 0,
			stepErr:  nil,
			envs:     map[string]string{"CI_CUSTOM_ERROR_CATEGORIZATION": "true"},
			expected: false,
		},
		{
			name:     "step failed but FF disabled",
			exitCode: 1,
			stepErr:  nil,
			envs:     map[string]string{"CI_CUSTOM_ERROR_CATEGORIZATION": "false"},
			expected: false,
		},
		{
			name:     "step failed but FF absent",
			exitCode: 1,
			stepErr:  nil,
			envs:     map[string]string{},
			expected: false,
		},
		{
			name:     "step failed with nil envs",
			exitCode: 1,
			stepErr:  nil,
			envs:     nil,
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := shouldCategorizeError(tc.exitCode, tc.stepErr, tc.envs)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestResolveErrorsYAMLPath(t *testing.T) {
	t.Run("env var takes priority over workspace files", func(t *testing.T) {
		tmpDir := t.TempDir()
		envFile := filepath.Join(tmpDir, "custom-errors.yaml")
		require.NoError(t, os.WriteFile(envFile, []byte("rules: []"), 0644))

		harnessDir := filepath.Join(tmpDir, "workspace", ".harness")
		require.NoError(t, os.MkdirAll(harnessDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(harnessDir, "errors.yaml"), []byte("rules: []"), 0644))

		envs := map[string]string{"HARNESS_ERRORS_YAML_PATH": envFile}
		result := resolveErrorsYAMLPath(filepath.Join(tmpDir, "workspace"), envs)
		assert.Equal(t, envFile, result)
	})

	t.Run("env var file not found falls through to workspace", func(t *testing.T) {
		tmpDir := t.TempDir()
		harnessDir := filepath.Join(tmpDir, ".harness")
		require.NoError(t, os.MkdirAll(harnessDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(harnessDir, "errors.yaml"), []byte("rules: []"), 0644))

		envs := map[string]string{"HARNESS_ERRORS_YAML_PATH": "/nonexistent/path.yaml"}
		result := resolveErrorsYAMLPath(tmpDir, envs)
		assert.Equal(t, filepath.Join(harnessDir, "errors.yaml"), result)
	})

	t.Run("finds errors.yaml in workspace", func(t *testing.T) {
		tmpDir := t.TempDir()
		harnessDir := filepath.Join(tmpDir, ".harness")
		require.NoError(t, os.MkdirAll(harnessDir, 0755))
		yamlPath := filepath.Join(harnessDir, "errors.yaml")
		require.NoError(t, os.WriteFile(yamlPath, []byte("rules: []"), 0644))

		result := resolveErrorsYAMLPath(tmpDir, map[string]string{})
		assert.Equal(t, yamlPath, result)
	})

	t.Run("falls back to errors.yml", func(t *testing.T) {
		tmpDir := t.TempDir()
		harnessDir := filepath.Join(tmpDir, ".harness")
		require.NoError(t, os.MkdirAll(harnessDir, 0755))
		ymlPath := filepath.Join(harnessDir, "errors.yml")
		require.NoError(t, os.WriteFile(ymlPath, []byte("rules: []"), 0644))

		result := resolveErrorsYAMLPath(tmpDir, map[string]string{})
		assert.Equal(t, ymlPath, result)
	})

	t.Run("prefers errors.yaml over errors.yml", func(t *testing.T) {
		tmpDir := t.TempDir()
		harnessDir := filepath.Join(tmpDir, ".harness")
		require.NoError(t, os.MkdirAll(harnessDir, 0755))
		yamlPath := filepath.Join(harnessDir, "errors.yaml")
		require.NoError(t, os.WriteFile(yamlPath, []byte("rules: []"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(harnessDir, "errors.yml"), []byte("rules: []"), 0644))

		result := resolveErrorsYAMLPath(tmpDir, map[string]string{})
		assert.Equal(t, yamlPath, result)
	})

	t.Run("returns empty when no yaml found", func(t *testing.T) {
		tmpDir := t.TempDir()
		result := resolveErrorsYAMLPath(tmpDir, map[string]string{})
		assert.Equal(t, "", result)
	})

	t.Run("returns empty when .harness dir exists but no yaml", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".harness"), 0755))
		result := resolveErrorsYAMLPath(tmpDir, map[string]string{})
		assert.Equal(t, "", result)
	})
}

func TestParseHcliOutput(t *testing.T) {
	tmpDir := t.TempDir()
	stdoutPath := filepath.Join(tmpDir, "stdout.log")
	stderrPath := filepath.Join(tmpDir, "stderr.log")
	require.NoError(t, os.WriteFile(stdoutPath, []byte("some stdout content here"), 0644))
	require.NoError(t, os.WriteFile(stderrPath, []byte("err"), 0644))

	t.Run("parses valid hcli JSON output", func(t *testing.T) {
		hcliOut := hcliEvaluateOutput{
			FailureType:    "APPLICATION_FAILURE",
			FailureSubType: "DEPENDENCY_RESOLUTION_FAILURE",
			Message:        "npm install failed: package not found",
			MatchedRule:    "npm-resolution-errors",
			Source:         "custom",
			RuleCount:      5,
		}
		output, err := json.Marshal(hcliOut)
		require.NoError(t, err)

		details, parseErr := parseHcliOutput(output, 42, stdoutPath, stderrPath)

		assert.NoError(t, parseErr)
		require.NotNil(t, details)
		assert.Equal(t, "APPLICATION_FAILURE", details.FailureType)
		assert.Equal(t, "DEPENDENCY_RESOLUTION_FAILURE", details.FailureSubType)
		assert.Equal(t, "npm install failed: package not found", details.Message)
		assert.Equal(t, "npm-resolution-errors", details.MatchedRule)
		assert.Equal(t, "custom", details.Source)
		assert.Equal(t, int64(42), details.EvaluationDurationMs)
		assert.Equal(t, 5, details.RuleCount)
		assert.False(t, details.TimedOut)
		assert.Equal(t, int64(24), details.StdoutSizeBytes) // len("some stdout content here")
		assert.Equal(t, int64(3), details.StderrSizeBytes)  // len("err")
	})

	t.Run("returns nil for empty output", func(t *testing.T) {
		details, err := parseHcliOutput([]byte{}, 10, stdoutPath, stderrPath)
		assert.NoError(t, err)
		assert.Nil(t, details)
	})

	t.Run("returns nil for empty JSON object with no meaningful fields", func(t *testing.T) {
		details, err := parseHcliOutput([]byte(`{}`), 10, stdoutPath, stderrPath)
		assert.NoError(t, err)
		assert.Nil(t, details)
	})

	t.Run("returns nil for JSON with only rule_count", func(t *testing.T) {
		details, err := parseHcliOutput([]byte(`{"rule_count": 3}`), 10, stdoutPath, stderrPath)
		assert.NoError(t, err)
		assert.Nil(t, details)
	})

	t.Run("returns error for invalid JSON", func(t *testing.T) {
		details, err := parseHcliOutput([]byte(`not json`), 10, stdoutPath, stderrPath)
		assert.Error(t, err)
		assert.Nil(t, details)
		assert.Contains(t, err.Error(), "unmarshal hcli output")
	})

	t.Run("handles missing log files gracefully", func(t *testing.T) {
		hcliOut := hcliEvaluateOutput{
			FailureType: "CONNECTIVITY_FAILURE",
			Message:     "connection refused",
			Source:      "custom",
		}
		output, err := json.Marshal(hcliOut)
		require.NoError(t, err)

		details, parseErr := parseHcliOutput(output, 5, "/nonexistent/stdout", "/nonexistent/stderr")

		assert.NoError(t, parseErr)
		require.NotNil(t, details)
		assert.Equal(t, "CONNECTIVITY_FAILURE", details.FailureType)
		assert.Equal(t, int64(0), details.StdoutSizeBytes)
		assert.Equal(t, int64(0), details.StderrSizeBytes)
	})

	t.Run("returns details when only message is set", func(t *testing.T) {
		output := []byte(`{"message": "something went wrong"}`)
		details, err := parseHcliOutput(output, 1, stdoutPath, stderrPath)
		assert.NoError(t, err)
		require.NotNil(t, details)
		assert.Equal(t, "something went wrong", details.Message)
	})

	t.Run("returns details when only matched_rule is set", func(t *testing.T) {
		output := []byte(`{"matched_rule": "my-rule"}`)
		details, err := parseHcliOutput(output, 1, stdoutPath, stderrPath)
		assert.NoError(t, err)
		require.NotNil(t, details)
		assert.Equal(t, "my-rule", details.MatchedRule)
	})
}

func TestGetCapturedOutputPath(t *testing.T) {
	path := getCapturedOutputPath("step-123")
	assert.Contains(t, path, "step-123-captured-output.log")
	assert.True(t, filepath.IsAbs(path))
}

func TestConvertStatusWithErrorDetails(t *testing.T) {
	t.Run("error details propagated to poll response", func(t *testing.T) {
		details := &api.ErrorDetails{
			FailureType:          "APPLICATION_FAILURE",
			FailureSubType:       "BUILD_FAILURE",
			Message:              "compilation failed",
			MatchedRule:          "build-errors",
			Source:               "custom",
			EvaluationDurationMs: 150,
			RuleCount:            3,
		}
		status := StepStatus{
			Status:       Complete,
			ErrorDetails: details,
		}

		resp := convertStatus(status)

		require.NotNil(t, resp.ErrorDetails)
		assert.Equal(t, "APPLICATION_FAILURE", resp.ErrorDetails.FailureType)
		assert.Equal(t, "BUILD_FAILURE", resp.ErrorDetails.FailureSubType)
		assert.Equal(t, "compilation failed", resp.ErrorDetails.Message)
		assert.Equal(t, "build-errors", resp.ErrorDetails.MatchedRule)
		assert.Equal(t, "custom", resp.ErrorDetails.Source)
		assert.Equal(t, int64(150), resp.ErrorDetails.EvaluationDurationMs)
		assert.Equal(t, 3, resp.ErrorDetails.RuleCount)
	})

	t.Run("nil error details preserved as nil", func(t *testing.T) {
		status := StepStatus{
			Status:       Complete,
			ErrorDetails: nil,
		}

		resp := convertStatus(status)
		assert.Nil(t, resp.ErrorDetails)
	})
}

func TestErrorDetailsJSONSerialization(t *testing.T) {
	t.Run("serializes all fields correctly", func(t *testing.T) {
		details := api.ErrorDetails{
			FailureType:          "CONNECTIVITY_FAILURE",
			FailureSubType:       "SOCKET_CONNECTION_FAILURE",
			Message:              "Could not reach registry.npmjs.org",
			MatchedRule:          "npm-connectivity",
			Source:               "custom",
			EvaluationDurationMs: 2500,
			StdoutSizeBytes:      1024,
			StderrSizeBytes:      512,
			RuleCount:            7,
			TimedOut:             false,
		}

		data, err := json.Marshal(details)
		require.NoError(t, err)

		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &parsed))

		assert.Equal(t, "CONNECTIVITY_FAILURE", parsed["failure_type"])
		assert.Equal(t, "SOCKET_CONNECTION_FAILURE", parsed["failure_sub_type"])
		assert.Equal(t, "Could not reach registry.npmjs.org", parsed["message"])
		assert.Equal(t, "npm-connectivity", parsed["matched_rule"])
		assert.Equal(t, "custom", parsed["source"])
		assert.Equal(t, float64(2500), parsed["evaluation_duration_ms"])
		assert.Equal(t, float64(1024), parsed["stdout_size_bytes"])
		assert.Equal(t, float64(512), parsed["stderr_size_bytes"])
		assert.Equal(t, float64(7), parsed["rule_count"])
	})

	t.Run("omits empty fields", func(t *testing.T) {
		details := api.ErrorDetails{
			TimedOut: true,
			Source:   "custom",
		}

		data, err := json.Marshal(details)
		require.NoError(t, err)

		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &parsed))

		_, hasFailureType := parsed["failure_type"]
		assert.False(t, hasFailureType, "empty failure_type should be omitted")
		_, hasMessage := parsed["message"]
		assert.False(t, hasMessage, "empty message should be omitted")

		assert.Equal(t, true, parsed["timed_out"])
		assert.Equal(t, "custom", parsed["source"])
	})

	t.Run("PollStepResponse includes error_details when present", func(t *testing.T) {
		resp := api.PollStepResponse{
			Exited:   true,
			ExitCode: 1,
			Error:    "exit status 1",
			ErrorDetails: &api.ErrorDetails{
				FailureType: "APPLICATION_FAILURE",
				Message:     "test failure",
				Source:      "custom",
			},
		}

		data, err := json.Marshal(resp)
		require.NoError(t, err)

		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &parsed))

		errorDetails, ok := parsed["error_details"].(map[string]interface{})
		require.True(t, ok, "error_details should be a JSON object")
		assert.Equal(t, "APPLICATION_FAILURE", errorDetails["failure_type"])
		assert.Equal(t, "test failure", errorDetails["message"])
		assert.Equal(t, "custom", errorDetails["source"])
	})

	t.Run("PollStepResponse omits error_details when nil", func(t *testing.T) {
		resp := api.PollStepResponse{
			Exited:   true,
			ExitCode: 0,
		}

		data, err := json.Marshal(resp)
		require.NoError(t, err)

		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &parsed))

		_, ok := parsed["error_details"]
		assert.False(t, ok, "error_details should be omitted when nil")
	})
}

func TestEvaluateErrorCategorization_NoHcli(t *testing.T) {
	// When hcli is not on PATH, evaluateErrorCategorization should return nil gracefully
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir()) // empty dir, no hcli

	tmpDir := t.TempDir()
	harnessDir := filepath.Join(tmpDir, ".harness")
	require.NoError(t, os.MkdirAll(harnessDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(harnessDir, "errors.yaml"), []byte("rules: []"), 0644))

	stdoutFile := filepath.Join(tmpDir, "stdout.log")
	require.NoError(t, os.WriteFile(stdoutFile, []byte("error output"), 0644))

	result := evaluateErrorCategorization(
		tmpDir, stdoutFile, 1, "step-1", "stage-1",
		map[string]string{"CI_CUSTOM_ERROR_CATEGORIZATION": "true"},
	)

	assert.Nil(t, result)

	// Restore PATH
	os.Setenv("PATH", origPath)
}

func TestEvaluateErrorCategorization_NoErrorsYAML(t *testing.T) {
	tmpDir := t.TempDir()
	stdoutFile := filepath.Join(tmpDir, "stdout.log")
	require.NoError(t, os.WriteFile(stdoutFile, []byte("error output"), 0644))

	result := evaluateErrorCategorization(
		tmpDir, stdoutFile, 1, "step-1", "stage-1",
		map[string]string{"CI_CUSTOM_ERROR_CATEGORIZATION": "true"},
	)

	assert.Nil(t, result)
}
