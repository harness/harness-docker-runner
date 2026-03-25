package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/harness/harness-docker-runner/api"
	"github.com/harness/harness-docker-runner/pipeline"
	"github.com/sirupsen/logrus"
)

const (
	errorCategorizationFF      = "CI_CUSTOM_ERROR_CATEGORIZATION"
	errorsYAMLPathEnv          = "HARNESS_ERRORS_YAML_PATH"
	evaluationTimeout          = 5 * time.Second
	hcliWindowsBinary          = "hcli.exe"
	hcliUnixBinary             = "hcli"
	capturedOutputSuffix       = "-captured-output.log"
	harnessInternalCacheSubdir = ".harness-internal/cache"
)

func getCapturedOutputPath(stepID string) string {
	return filepath.Join(pipeline.GetSharedVolPath(), stepID+capturedOutputSuffix)
}

func isErrorCategorizationEnabled(envs map[string]string) bool {
	val, ok := envs[errorCategorizationFF]
	return ok && strings.EqualFold(val, "true")
}

func shouldCategorizeError(exitCode int, stepErr error, envs map[string]string) bool {
	hasFailed := stepErr != nil || exitCode != 0
	if !hasFailed {
		return false
	}
	return isErrorCategorizationEnabled(envs)
}

func resolveHcliBinaryPath() string {
	binary := hcliUnixBinary
	if runtime.GOOS == "windows" {
		binary = hcliWindowsBinary
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		logrus.WithField("binary", binary).Warnln("hcli binary not found in PATH, skipping error categorization")
		return ""
	}
	return path
}

func resolveErrorsYAMLPath(workingDir string, envs map[string]string) string {
	if envPath, ok := envs[errorsYAMLPathEnv]; ok && envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath
		}
		logrus.WithField("path", envPath).Warnln("HARNESS_ERRORS_YAML_PATH set but file not found")
	}

	yamlPath := filepath.Join(workingDir, ".harness", "errors.yaml")
	if _, err := os.Stat(yamlPath); err == nil {
		return yamlPath
	}

	ymlPath := filepath.Join(workingDir, ".harness", "errors.yml")
	if _, err := os.Stat(ymlPath); err == nil {
		return ymlPath
	}

	return ""
}

// evaluateErrorCategorization invokes hcli errors evaluate and returns structured
// error details. It enforces a hard timeout and recovers from panics to guarantee
// it never blocks step completion.
func evaluateErrorCategorization(
	workingDir string,
	capturedOutputPath string,
	exitCode int,
	stepID string,
	stageID string,
	envs map[string]string,
) *api.ErrorDetails {
	start := time.Now()

	type result struct {
		details *api.ErrorDetails
	}
	ch := make(chan result, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logrus.WithField("panic", r).Warnln("recovered from panic during error categorization")
				ch <- result{details: nil}
			}
		}()

		hcliPath := resolveHcliBinaryPath()
		if hcliPath == "" {
			ch <- result{details: nil}
			return
		}

		yamlPath := resolveErrorsYAMLPath(workingDir, envs)
		if yamlPath == "" {
			logrus.Infoln("no errors.yaml found, skipping error categorization")
			ch <- result{details: nil}
			return
		}

		cacheDir := filepath.Join(workingDir, harnessInternalCacheSubdir)
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			logrus.WithError(err).Warnln("failed to create cache dir for error categorization")
		}

		pipelineID := envs["HARNESS_PIPELINE_ID"]

		stderrPath := capturedOutputPath + ".stderr"
		if err := os.WriteFile(stderrPath, []byte{}, 0644); err != nil {
			logrus.WithError(err).Warnln("failed to create empty stderr file for error categorization")
			stderrPath = capturedOutputPath
		}
		defer os.Remove(stderrPath)

		ctx, cancel := context.WithTimeout(context.Background(), evaluationTimeout)
		defer cancel()

		args := []string{
			"errors", "evaluate",
			"--yaml-path", yamlPath,
			"--stdout-path", capturedOutputPath,
			"--stderr-path", stderrPath,
			"--exit-code", strconv.Itoa(exitCode),
			"--step-id", stepID,
			"--stage-id", stageID,
			"--pipeline-id", pipelineID,
			"--cache-dir", cacheDir,
		}

		logrus.WithFields(logrus.Fields{
			"hcli":    hcliPath,
			"yaml":    yamlPath,
			"step_id": stepID,
		}).Infoln("invoking hcli errors evaluate")

		cmd := exec.CommandContext(ctx, hcliPath, args...)
		output, err := cmd.Output()

		durationMs := time.Since(start).Milliseconds()

		if ctx.Err() == context.DeadlineExceeded {
			logrus.WithField("step_id", stepID).Warnln("error categorization timed out")
			ch <- result{details: &api.ErrorDetails{
				TimedOut:             true,
				EvaluationDurationMs: durationMs,
				Source:               "custom",
			}}
			return
		}

		if err != nil {
			logrus.WithError(err).WithField("step_id", stepID).Warnln("hcli errors evaluate failed")
			ch <- result{details: nil}
			return
		}

		details, parseErr := parseHcliOutput(output, durationMs, capturedOutputPath, stderrPath)
		if parseErr != nil {
			logrus.WithError(parseErr).Warnln("failed to parse hcli errors evaluate output")
			ch <- result{details: nil}
			return
		}

		ch <- result{details: details}
	}()

	select {
	case res := <-ch:
		return res.details
	case <-time.After(evaluationTimeout + 2*time.Second):
		logrus.WithField("step_id", stepID).Warnln("error categorization safety timeout exceeded")
		return &api.ErrorDetails{
			TimedOut:             true,
			EvaluationDurationMs: time.Since(start).Milliseconds(),
			Source:               "custom",
		}
	}
}

// hcliEvaluateOutput represents the JSON output from hcli errors evaluate.
type hcliEvaluateOutput struct {
	FailureType    string `json:"failure_type"`
	FailureSubType string `json:"failure_sub_type"`
	Message        string `json:"message"`
	MatchedRule    string `json:"matched_rule"`
	Source         string `json:"source"`
	RuleCount      int    `json:"rule_count"`
}

func parseHcliOutput(output []byte, durationMs int64, stdoutPath, stderrPath string) (*api.ErrorDetails, error) {
	if len(output) == 0 {
		return nil, nil
	}

	var hcliOut hcliEvaluateOutput
	if err := json.Unmarshal(output, &hcliOut); err != nil {
		return nil, fmt.Errorf("unmarshal hcli output: %w", err)
	}

	if hcliOut.FailureType == "" && hcliOut.Message == "" && hcliOut.MatchedRule == "" {
		return nil, nil
	}

	var stdoutSize, stderrSize int64
	if info, err := os.Stat(stdoutPath); err == nil {
		stdoutSize = info.Size()
	}
	if info, err := os.Stat(stderrPath); err == nil {
		stderrSize = info.Size()
	}

	return &api.ErrorDetails{
		FailureType:          hcliOut.FailureType,
		FailureSubType:       hcliOut.FailureSubType,
		Message:              hcliOut.Message,
		MatchedRule:          hcliOut.MatchedRule,
		Source:               hcliOut.Source,
		EvaluationDurationMs: durationMs,
		StdoutSizeBytes:      stdoutSize,
		StderrSizeBytes:      stderrSize,
		RuleCount:            hcliOut.RuleCount,
		TimedOut:             false,
	}, nil
}
