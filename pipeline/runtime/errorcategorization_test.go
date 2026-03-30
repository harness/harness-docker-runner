package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/harness/harness-docker-runner/api"
	"github.com/harness/harness-docker-runner/engine/spec"
	tiCfg "github.com/harness/lite-engine/ti/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestTiConfig(pipelineID, stageID string) *tiCfg.Cfg {
	cfg := tiCfg.New(
		"", "", "", "", "",
		pipelineID, "", stageID,
		"", "", "",
		"", "", "",
		"", "",
		false, false, "", "",
	)
	return &cfg
}

func TestShouldCategorizeError(t *testing.T) {
	t.Run("success never categorizes even with nil engine", func(t *testing.T) {
		assert.False(t, shouldCategorizeError(0, nil, nil))
	})
	t.Run("failure with nil engine returns false (FF disabled)", func(t *testing.T) {
		assert.False(t, shouldCategorizeError(1, nil, nil))
	})
	t.Run("failure with error and nil engine returns false", func(t *testing.T) {
		assert.False(t, shouldCategorizeError(0, fmt.Errorf("err"), nil))
	})
}

func TestIsFeatureFlagEnabled(t *testing.T) {
	t.Run("nil engine and nil step returns false", func(t *testing.T) {
		assert.False(t, IsFeatureFlagEnabled(errorCategorizationFF, nil, nil))
	})
	t.Run("nil engine with step containing FF returns true", func(t *testing.T) {
		step := &spec.Step{Envs: map[string]string{errorCategorizationFF: "true"}}
		assert.True(t, IsFeatureFlagEnabled(errorCategorizationFF, nil, step))
	})
	t.Run("nil engine with step containing wrong value returns false", func(t *testing.T) {
		step := &spec.Step{Envs: map[string]string{errorCategorizationFF: "false"}}
		assert.False(t, IsFeatureFlagEnabled(errorCategorizationFF, nil, step))
	})
	t.Run("nil engine with step missing FF returns false", func(t *testing.T) {
		step := &spec.Step{Envs: map[string]string{}}
		assert.False(t, IsFeatureFlagEnabled(errorCategorizationFF, nil, step))
	})
}

func TestResolveStageID(t *testing.T) {
	tests := []struct {
		name           string
		stageRuntimeID string
		envs           map[string]string
		tiConfig       *tiCfg.Cfg
		expected       string
	}{
		{
			name:           "env var takes highest priority",
			stageRuntimeID: "runtime-stage",
			envs:           map[string]string{"HARNESS_STAGE_ID": "env-stage"},
			tiConfig:       newTestTiConfig("", "ti-stage"),
			expected:       "env-stage",
		},
		{
			name:           "tiConfig used when env var missing",
			stageRuntimeID: "runtime-stage",
			envs:           map[string]string{},
			tiConfig:       newTestTiConfig("", "ti-stage"),
			expected:       "ti-stage",
		},
		{
			name:           "stageRuntimeID fallback when both missing",
			stageRuntimeID: "runtime-stage",
			envs:           map[string]string{},
			tiConfig:       nil,
			expected:       "runtime-stage",
		},
		{
			name:           "stageRuntimeID fallback when tiConfig has empty stageID",
			stageRuntimeID: "runtime-stage",
			envs:           map[string]string{},
			tiConfig:       newTestTiConfig("", ""),
			expected:       "runtime-stage",
		},
		{
			name:           "empty env var skipped",
			stageRuntimeID: "runtime-stage",
			envs:           map[string]string{"HARNESS_STAGE_ID": ""},
			tiConfig:       newTestTiConfig("", "ti-stage"),
			expected:       "ti-stage",
		},
		{
			name:           "nil envs uses tiConfig",
			stageRuntimeID: "runtime-stage",
			envs:           nil,
			tiConfig:       newTestTiConfig("", "ti-stage"),
			expected:       "ti-stage",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := resolveStageID(tc.stageRuntimeID, tc.envs, tc.tiConfig)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestResolvePipelineID(t *testing.T) {
	tests := []struct {
		name     string
		envs     map[string]string
		tiConfig *tiCfg.Cfg
		expected string
	}{
		{
			name:     "env var takes priority",
			envs:     map[string]string{"HARNESS_PIPELINE_ID": "env-pipeline"},
			tiConfig: newTestTiConfig("ti-pipeline", ""),
			expected: "env-pipeline",
		},
		{
			name:     "tiConfig used when env var missing",
			envs:     map[string]string{},
			tiConfig: newTestTiConfig("ti-pipeline", ""),
			expected: "ti-pipeline",
		},
		{
			name:     "empty string when both missing",
			envs:     map[string]string{},
			tiConfig: nil,
			expected: "",
		},
		{
			name:     "empty env var skipped",
			envs:     map[string]string{"HARNESS_PIPELINE_ID": ""},
			tiConfig: newTestTiConfig("ti-pipeline", ""),
			expected: "ti-pipeline",
		},
		{
			name:     "nil envs uses tiConfig",
			envs:     nil,
			tiConfig: newTestTiConfig("ti-pipeline", ""),
			expected: "ti-pipeline",
		},
		{
			name:     "empty tiConfig pipelineID returns empty",
			envs:     map[string]string{},
			tiConfig: newTestTiConfig("", ""),
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := resolvePipelineID(tc.envs, tc.tiConfig)
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
		result := resolveErrorsYAMLPath(filepath.Join(tmpDir, "workspace"), envs, nil)
		assert.Equal(t, envFile, result)
	})

	t.Run("env var file not found falls through to workspace", func(t *testing.T) {
		tmpDir := t.TempDir()
		harnessDir := filepath.Join(tmpDir, ".harness")
		require.NoError(t, os.MkdirAll(harnessDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(harnessDir, "errors.yaml"), []byte("rules: []"), 0644))

		envs := map[string]string{"HARNESS_ERRORS_YAML_PATH": "/nonexistent/path.yaml"}
		result := resolveErrorsYAMLPath(tmpDir, envs, nil)
		assert.Equal(t, filepath.Join(harnessDir, "errors.yaml"), result)
	})

	t.Run("finds errors.yaml in workspace", func(t *testing.T) {
		tmpDir := t.TempDir()
		harnessDir := filepath.Join(tmpDir, ".harness")
		require.NoError(t, os.MkdirAll(harnessDir, 0755))
		yamlPath := filepath.Join(harnessDir, "errors.yaml")
		require.NoError(t, os.WriteFile(yamlPath, []byte("rules: []"), 0644))

		result := resolveErrorsYAMLPath(tmpDir, map[string]string{}, nil)
		assert.Equal(t, yamlPath, result)
	})

	t.Run("falls back to errors.yml", func(t *testing.T) {
		tmpDir := t.TempDir()
		harnessDir := filepath.Join(tmpDir, ".harness")
		require.NoError(t, os.MkdirAll(harnessDir, 0755))
		ymlPath := filepath.Join(harnessDir, "errors.yml")
		require.NoError(t, os.WriteFile(ymlPath, []byte("rules: []"), 0644))

		result := resolveErrorsYAMLPath(tmpDir, map[string]string{}, nil)
		assert.Equal(t, ymlPath, result)
	})

	t.Run("prefers errors.yaml over errors.yml", func(t *testing.T) {
		tmpDir := t.TempDir()
		harnessDir := filepath.Join(tmpDir, ".harness")
		require.NoError(t, os.MkdirAll(harnessDir, 0755))
		yamlPath := filepath.Join(harnessDir, "errors.yaml")
		require.NoError(t, os.WriteFile(yamlPath, []byte("rules: []"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(harnessDir, "errors.yml"), []byte("rules: []"), 0644))

		result := resolveErrorsYAMLPath(tmpDir, map[string]string{}, nil)
		assert.Equal(t, yamlPath, result)
	})

	t.Run("returns empty when no yaml found", func(t *testing.T) {
		tmpDir := t.TempDir()
		result := resolveErrorsYAMLPath(tmpDir, map[string]string{}, nil)
		assert.Equal(t, "", result)
	})

	t.Run("returns empty when .harness dir exists but no yaml", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".harness"), 0755))
		result := resolveErrorsYAMLPath(tmpDir, map[string]string{}, nil)
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

func TestLogFilePaths(t *testing.T) {
	t.Run("stdout path follows K8s convention", func(t *testing.T) {
		path := getStdoutLogFilePath("step-123")
		assert.Contains(t, path, ".harness-internal/logs/step-123-stdout.log")
		assert.True(t, filepath.IsAbs(path))
	})

	t.Run("stderr path follows K8s convention", func(t *testing.T) {
		path := getStderrLogFilePath("step-123")
		assert.Contains(t, path, ".harness-internal/logs/step-123-stderr.log")
		assert.True(t, filepath.IsAbs(path))
	})

	t.Run("paths are distinct for same step", func(t *testing.T) {
		stdout := getStdoutLogFilePath("step-abc")
		stderr := getStderrLogFilePath("step-abc")
		assert.NotEqual(t, stdout, stderr)
	})

	t.Run("paths are step-scoped", func(t *testing.T) {
		p1 := getStdoutLogFilePath("step-1")
		p2 := getStdoutLogFilePath("step-2")
		assert.NotEqual(t, p1, p2)
		assert.Contains(t, p1, "step-1")
		assert.Contains(t, p2, "step-2")
	})
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
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())

	tmpDir := t.TempDir()
	harnessDir := filepath.Join(tmpDir, ".harness")
	require.NoError(t, os.MkdirAll(harnessDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(harnessDir, "errors.yaml"), []byte("rules: []"), 0644))

	stdoutFile := filepath.Join(tmpDir, "stdout.log")
	stderrFile := filepath.Join(tmpDir, "stderr.log")
	require.NoError(t, os.WriteFile(stdoutFile, []byte("error output"), 0644))
	require.NoError(t, os.WriteFile(stderrFile, []byte(""), 0644))

	result := evaluateErrorCategorization(
		tmpDir, stdoutFile, stderrFile, 1, "step-1", "stage-1",
		map[string]string{"CI_CUSTOM_ERROR_CATEGORIZATION": "true"},
		nil, nil,
	)

	assert.Nil(t, result)

	os.Setenv("PATH", origPath)
}

func TestEvaluateErrorCategorization_NoErrorsYAML(t *testing.T) {
	tmpDir := t.TempDir()
	stdoutFile := filepath.Join(tmpDir, "stdout.log")
	stderrFile := filepath.Join(tmpDir, "stderr.log")
	require.NoError(t, os.WriteFile(stdoutFile, []byte("error output"), 0644))
	require.NoError(t, os.WriteFile(stderrFile, []byte(""), 0644))

	result := evaluateErrorCategorization(
		tmpDir, stdoutFile, stderrFile, 1, "step-1", "stage-1",
		map[string]string{"CI_CUSTOM_ERROR_CATEGORIZATION": "true"},
		nil, nil,
	)

	assert.Nil(t, result)
}

func TestEvaluateErrorCategorization_WithTiConfig(t *testing.T) {
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())

	tmpDir := t.TempDir()
	harnessDir := filepath.Join(tmpDir, ".harness")
	require.NoError(t, os.MkdirAll(harnessDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(harnessDir, "errors.yaml"), []byte("rules: []"), 0644))

	stdoutFile := filepath.Join(tmpDir, "stdout.log")
	stderrFile := filepath.Join(tmpDir, "stderr.log")
	require.NoError(t, os.WriteFile(stdoutFile, []byte("error output"), 0644))
	require.NoError(t, os.WriteFile(stderrFile, []byte("stderr output"), 0644))

	ti := newTestTiConfig("my-pipeline", "my-stage")

	result := evaluateErrorCategorization(
		tmpDir, stdoutFile, stderrFile, 1, "step-1", "fallback-stage",
		map[string]string{},
		ti, nil,
	)

	// hcli not on PATH, so result is nil; but the function should not panic
	assert.Nil(t, result)

	os.Setenv("PATH", origPath)
}

func TestCleanupLogFiles(t *testing.T) {
	t.Run("deletes files that exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		stdoutPath := filepath.Join(tmpDir, "step-1-stdout.log")
		stderrPath := filepath.Join(tmpDir, "step-1-stderr.log")
		require.NoError(t, os.WriteFile(stdoutPath, []byte("stdout data"), 0644))
		require.NoError(t, os.WriteFile(stderrPath, []byte("stderr data"), 0644))

		cleanupLogFiles(stdoutPath, stderrPath)

		_, err := os.Stat(stdoutPath)
		assert.True(t, os.IsNotExist(err), "stdout file should be deleted")
		_, err = os.Stat(stderrPath)
		assert.True(t, os.IsNotExist(err), "stderr file should be deleted")
	})

	t.Run("idempotent - second call on already-deleted files is silent", func(t *testing.T) {
		tmpDir := t.TempDir()
		stdoutPath := filepath.Join(tmpDir, "step-2-stdout.log")
		stderrPath := filepath.Join(tmpDir, "step-2-stderr.log")
		require.NoError(t, os.WriteFile(stdoutPath, []byte("data"), 0644))
		require.NoError(t, os.WriteFile(stderrPath, []byte("data"), 0644))

		cleanupLogFiles(stdoutPath, stderrPath)
		// Second call should not panic or log errors
		cleanupLogFiles(stdoutPath, stderrPath)

		_, err := os.Stat(stdoutPath)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("silent on files that never existed", func(t *testing.T) {
		// Should not panic when files were never created
		cleanupLogFiles("/nonexistent/path/stdout.log", "/nonexistent/path/stderr.log")
	})

	t.Run("handles partial existence - only stdout exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		stdoutPath := filepath.Join(tmpDir, "step-3-stdout.log")
		stderrPath := filepath.Join(tmpDir, "step-3-stderr.log")
		require.NoError(t, os.WriteFile(stdoutPath, []byte("stdout only"), 0644))

		cleanupLogFiles(stdoutPath, stderrPath)

		_, err := os.Stat(stdoutPath)
		assert.True(t, os.IsNotExist(err), "stdout file should be deleted")
	})

	t.Run("handles partial existence - only stderr exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		stdoutPath := filepath.Join(tmpDir, "step-4-stdout.log")
		stderrPath := filepath.Join(tmpDir, "step-4-stderr.log")
		require.NoError(t, os.WriteFile(stderrPath, []byte("stderr only"), 0644))

		cleanupLogFiles(stdoutPath, stderrPath)

		_, err := os.Stat(stderrPath)
		assert.True(t, os.IsNotExist(err), "stderr file should be deleted")
	})
}
