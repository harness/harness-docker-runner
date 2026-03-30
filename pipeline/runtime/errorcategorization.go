package runtime

import (
	"bytes"
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
	"github.com/harness/harness-docker-runner/engine"
	"github.com/harness/harness-docker-runner/pipeline"
	tiCfg "github.com/harness/lite-engine/ti/config"
	"github.com/sirupsen/logrus"
)

const (
	errorCategorizationFF      = "CI_CUSTOM_ERROR_CATEGORIZATION"
	errorsYAMLPathEnv          = "HARNESS_ERRORS_YAML_PATH"
	stageIDEnv                 = "HARNESS_STAGE_ID"
	pipelineIDEnv              = "HARNESS_PIPELINE_ID"
	evaluationTimeout          = 5 * time.Second
	hcliWindowsBinary          = "hcli.exe"
	hcliUnixBinary             = "hcli"
	stdoutLogSuffix            = "-stdout.log"
	stderrLogSuffix            = "-stderr.log"
	harnessInternalLogsSubdir  = ".harness-internal/logs"
	harnessInternalCacheSubdir = ".harness-internal/cache"
)

func getStdoutLogFilePath(stepID string) string {
	return filepath.Join(pipeline.GetSharedVolPath(), harnessInternalLogsSubdir, stepID+stdoutLogSuffix)
}

func getStderrLogFilePath(stepID string) string {
	return filepath.Join(pipeline.GetSharedVolPath(), harnessInternalLogsSubdir, stepID+stderrLogSuffix)
}

func ensureLogDir() {
	dir := filepath.Join(pipeline.GetSharedVolPath(), harnessInternalLogsSubdir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		logrus.WithError(err).Warnln("failed to create log capture directory")
	}
}

// cleanupLogFiles removes the stdout and stderr capture files. It is idempotent:
// already-deleted or never-created files are silently ignored.
func cleanupLogFiles(stdoutPath, stderrPath string) {
	if err := os.Remove(stdoutPath); err != nil && !os.IsNotExist(err) {
		logrus.WithError(err).WithField("path", stdoutPath).Warnln("failed to remove stdout log file")
	}
	if err := os.Remove(stderrPath); err != nil && !os.IsNotExist(err) {
		logrus.WithError(err).WithField("path", stderrPath).Warnln("failed to remove stderr log file")
	}
}

func resolveStageID(stageRuntimeID string, envs map[string]string, tiConfig *tiCfg.Cfg) string {
	if v, ok := envs[stageIDEnv]; ok && v != "" {
		return v
	}
	if tiConfig != nil {
		if id := tiConfig.GetStageID(); id != "" {
			return id
		}
	}
	return stageRuntimeID
}

func resolvePipelineID(envs map[string]string, tiConfig *tiCfg.Cfg) string {
	if v, ok := envs[pipelineIDEnv]; ok && v != "" {
		return v
	}
	if tiConfig != nil {
		if id := tiConfig.GetPipelineID(); id != "" {
			return id
		}
	}
	return ""
}

func shouldCategorizeError(exitCode int, stepErr error, eng *engine.Engine) bool {
	hasFailed := stepErr != nil || exitCode != 0
	if !hasFailed {
		return false
	}
	return IsFeatureFlagEnabled(errorCategorizationFF, eng, nil)
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

func resolveErrorsYAMLPath(workingDir string, envs map[string]string, eng *engine.Engine) string {
	envPath := ""
	if v, ok := envs[errorsYAMLPathEnv]; ok && v != "" {
		envPath = v
	} else if eng != nil {
		if v, ok := eng.GetPipelineEnv(errorsYAMLPathEnv); ok && v != "" {
			envPath = v
		}
	}
	if envPath != "" {
		// Try absolute first
		if _, err := os.Stat(envPath); err == nil {
			return envPath
		}
		// Try relative to workspace
		relative := filepath.Join(workingDir, envPath)
		if _, err := os.Stat(relative); err == nil {
			return relative
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
	stdoutPath string,
	stderrPath string,
	exitCode int,
	stepID string,
	stageRuntimeID string,
	envs map[string]string,
	tiConfig *tiCfg.Cfg,
	eng *engine.Engine,
) *api.ErrorDetails {
	start := time.Now()

	stageID := resolveStageID(stageRuntimeID, envs, tiConfig)
	pipelineID := resolvePipelineID(envs, tiConfig)

	ctx, cancel := context.WithTimeout(context.Background(), evaluationTimeout)
	defer cancel()

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

		yamlPath := resolveErrorsYAMLPath(workingDir, envs, eng)
		if yamlPath == "" {
			logrus.Infoln("no errors.yaml found, skipping error categorization")
			ch <- result{details: nil}
			return
		}

		cacheDir := filepath.Join(workingDir, harnessInternalCacheSubdir)
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			logrus.WithError(err).Warnln("failed to create cache dir for error categorization")
		}

		args := []string{
			"errors", "evaluate",
			"--yaml-path", yamlPath,
			"--stdout-path", stdoutPath,
			"--stderr-path", stderrPath,
			"--exit-code", strconv.Itoa(exitCode),
			"--step-id", stepID,
			"--stage-id", stageID,
			"--pipeline-id", pipelineID,
			"--cache-dir", cacheDir,
		}

		logrus.WithFields(logrus.Fields{
			"hcli":        hcliPath,
			"yaml":        yamlPath,
			"step_id":     stepID,
			"stage_id":    stageID,
			"pipeline_id": pipelineID,
		}).Infoln("invoking hcli errors evaluate")

		cmd := exec.CommandContext(ctx, hcliPath, args...)
		var stderrBuf bytes.Buffer
		cmd.Stderr = &stderrBuf
		output, err := cmd.Output()

		durationMs := time.Since(start).Milliseconds()

		if ctx.Err() != nil {
			logrus.WithFields(logrus.Fields{
				"step_id":     stepID,
				"duration_ms": durationMs,
			}).Warnln("hcli errors evaluate timed out")
			ch <- result{details: &api.ErrorDetails{
				TimedOut:             true,
				EvaluationDurationMs: durationMs,
				Source:               "custom",
			}}
			return
		}

		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"step_id":     stepID,
				"duration_ms": durationMs,
				"stderr":      strings.TrimSpace(stderrBuf.String()),
			}).Warnln("hcli errors evaluate failed")
			ch <- result{details: nil}
			return
		}

		details, parseErr := parseHcliOutput(output, durationMs, stdoutPath, stderrPath)
		if parseErr != nil {
			logrus.WithError(parseErr).WithFields(logrus.Fields{
				"step_id":    stepID,
				"raw_output": strings.TrimSpace(string(output)),
				"stderr":     strings.TrimSpace(stderrBuf.String()),
			}).Warnln("failed to parse hcli errors evaluate output")
			ch <- result{details: nil}
			return
		}

		if details != nil {
			logrus.WithFields(logrus.Fields{
				"step_id":      stepID,
				"duration_ms":  durationMs,
				"matched_rule": details.MatchedRule,
				"failure_type": details.FailureType,
				"stdout_bytes": details.StdoutSizeBytes,
				"stderr_bytes": details.StderrSizeBytes,
				"rule_count":   details.RuleCount,
			}).Infoln("error categorization completed")
		} else {
			logrus.WithFields(logrus.Fields{
				"step_id":     stepID,
				"duration_ms": durationMs,
			}).Infoln("error categorization completed with no match")
		}

		ch <- result{details: details}
	}()

	select {
	case res := <-ch:
		return res.details
	case <-ctx.Done():
		logrus.WithFields(logrus.Fields{
			"step_id":     stepID,
			"duration_ms": time.Since(start).Milliseconds(),
		}).Warnln("error categorization timed out waiting for result")
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
	Matched        bool   `json:"matched"`
	TimedOut       bool   `json:"timed_out"`
	Error          string `json:"error"`
}

func parseHcliOutput(output []byte, durationMs int64, stdoutPath, stderrPath string) (*api.ErrorDetails, error) {
	if len(output) == 0 {
		return nil, nil
	}

	var hcliOut hcliEvaluateOutput
	if err := json.Unmarshal(output, &hcliOut); err != nil {
		return nil, fmt.Errorf("unmarshal hcli output: %w", err)
	}

	if hcliOut.Error != "" {
		logrus.WithField("hcli_error", hcliOut.Error).Warnln("hcli reported evaluation error")
	}

	if !hcliOut.Matched && !hcliOut.TimedOut {
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
		TimedOut:             hcliOut.TimedOut,
	}, nil
}
