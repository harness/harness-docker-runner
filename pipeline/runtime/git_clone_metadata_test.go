// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/drone/runner-go/pipeline/runtime"
	"github.com/harness/harness-docker-runner/api"
	"github.com/harness/harness-docker-runner/engine"
	"github.com/harness/harness-docker-runner/engine/spec"
	"github.com/harness/harness-docker-runner/logstream"
	"github.com/harness/harness-docker-runner/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type nopLogClient struct{}

func (nopLogClient) Upload(context.Context, string, []*logstream.Line) error { return nil }
func (nopLogClient) Open(context.Context, string) error                      { return nil }
func (nopLogClient) Close(context.Context, string) error                     { return nil }
func (nopLogClient) Write(context.Context, string, []*logstream.Line) error  { return nil }

func hostEngine(t *testing.T) *engine.Engine {
	t.Helper()
	disableDocker := false
	eng := &engine.Engine{}
	require.NoError(t, eng.Setup(context.Background(), &spec.PipelineConfig{EnableDockerSetup: &disableDocker}))
	return eng
}

func hideScript() *bool {
	v := false
	return &v
}

const sampleGitCloneReceipt = `{"repoUrl":"https://github.com/acme/app.git","commitSha":"abc123","refType":"pr","prNumber":17,"linesOfCode":48210,"branch":"main","tag":"v1.2.3","repoName":"app","orgName":"acme","timestamp":1732040000000,"scmType":"GITHUB"}`

func gitCloneReceiptPath(stepID string) string {
	return fmt.Sprintf("%s/%s-git-clone-metadata.json", pipeline.GetSharedVolPath(), stepID)
}

func writeGitCloneReceipt(t *testing.T, stepID, content string) {
	t.Helper()
	path := gitCloneReceiptPath(stepID)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	t.Cleanup(func() { _ = os.Remove(path) })
}

func TestReadGitCloneMetadata_FilePresent_ReturnsRawJSON(t *testing.T) {
	writeGitCloneReceipt(t, "gc-present", sampleGitCloneReceipt)
	assert.Equal(t, sampleGitCloneReceipt, readGitCloneMetadata("gc-present"))
}

func TestReadGitCloneMetadata_FileMissing_ReturnsEmpty(t *testing.T) {
	assert.Empty(t, readGitCloneMetadata("gc-does-not-exist"))
}

func TestReadGitCloneMetadata_EmptyFile_ReturnsEmpty(t *testing.T) {
	writeGitCloneReceipt(t, "gc-empty", "")
	assert.Empty(t, readGitCloneMetadata("gc-empty"))
}

func TestReadGitCloneMetadata_IsolatedPerStep(t *testing.T) {
	writeGitCloneReceipt(t, "gc-a", `{"commitSha":"aaa"}`)
	writeGitCloneReceipt(t, "gc-b", `{"commitSha":"bbb"}`)

	a := readGitCloneMetadata("gc-a")
	b := readGitCloneMetadata("gc-b")
	assert.Contains(t, a, "aaa")
	assert.NotContains(t, a, "bbb")
	assert.Contains(t, b, "bbb")
	assert.NotContains(t, b, "aaa")
}

func TestReadGitCloneMetadata_DoesNotParse(t *testing.T) {
	// The runner is a transport only; the manager parses. Malformed content must pass through.
	writeGitCloneReceipt(t, "gc-raw", "not json at all")
	assert.Equal(t, "not json at all", readGitCloneMetadata("gc-raw"))
}

func TestConvertStatus_CarriesGitCloneMetadata(t *testing.T) {
	resp := convertStatus(StepStatus{
		Status:           Complete,
		State:            &runtime.State{Exited: true, ExitCode: 0},
		GitCloneMetadata: sampleGitCloneReceipt,
	})
	assert.Equal(t, sampleGitCloneReceipt, resp.GitCloneMetadata)
}

func TestConvertStatus_NoMetadata_LeavesFieldEmpty(t *testing.T) {
	resp := convertStatus(StepStatus{Status: Complete, State: &runtime.State{Exited: true}})
	assert.Empty(t, resp.GitCloneMetadata)
}

func TestConvertStatus_FailedStep_StillCarriesMetadata(t *testing.T) {
	// Current behaviour: forwarded regardless of exit code; the manager only consumes it on success.
	resp := convertStatus(StepStatus{
		Status:           Complete,
		State:            &runtime.State{Exited: true, ExitCode: 1},
		StepErr:          fmt.Errorf("boom"),
		GitCloneMetadata: sampleGitCloneReceipt,
	})
	assert.Equal(t, 255, resp.ExitCode)
	assert.Equal(t, sampleGitCloneReceipt, resp.GitCloneMetadata)
}

func TestPollStepResponse_JSONKey_GitCloneMetadata(t *testing.T) {
	// The delegate's ExecuteStepResponse binds to exactly this wire key.
	with, err := json.Marshal(convertStatus(StepStatus{GitCloneMetadata: sampleGitCloneReceipt}))
	require.NoError(t, err)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(with, &m))
	assert.Equal(t, sampleGitCloneReceipt, m["git_clone_metadata"])
	assert.NotContains(t, string(with), "explicit_git_clone_metadata")

	without, err := json.Marshal(convertStatus(StepStatus{}))
	require.NoError(t, err)
	assert.NotContains(t, string(without), "git_clone_metadata")
}

func TestExecuteRunStep_SetsGitCloneMetadataFileEnvWhenMissing(t *testing.T) {
	require.NoError(t, os.MkdirAll(pipeline.GetSharedVolPath(), 0o755))
	id := "gc-env-set"
	req := &api.StartStepRequest{
		StartStepRequestConfig: api.StartStepRequestConfig{
			ID:                        id,
			Kind:                      api.Run,
			Envs:                      map[string]string{},
			ShowScriptInExecutionLogs: hideScript(),
			Run: api.RunConfig{
				Entrypoint: []string{"/bin/sh", "-c"},
				Command:    []string{"printenv HARNESS_GIT_CLONE_METADATA_FILE"},
			},
		},
	}

	_, _, _, _, _, _, err := executeRunStep(context.Background(), hostEngine(t), req, io.Discard, newTestTiConfig("p", "s"), nil)
	require.NoError(t, err)
	assert.Equal(t, gitCloneReceiptPath(id), req.Envs["HARNESS_GIT_CLONE_METADATA_FILE"])
}

func TestExecuteRunStep_DoesNotOverwriteGitCloneMetadataFileEnv(t *testing.T) {
	preset := "/custom/path/receipt.json"
	req := &api.StartStepRequest{
		StartStepRequestConfig: api.StartStepRequestConfig{
			ID:                        "gc-env-keep",
			Kind:                      api.Run,
			Envs:                      map[string]string{"HARNESS_GIT_CLONE_METADATA_FILE": preset},
			ShowScriptInExecutionLogs: hideScript(),
			Run: api.RunConfig{
				Entrypoint: []string{"/bin/sh", "-c"},
				Command:    []string{"true"},
			},
		},
	}

	_, _, _, _, _, _, err := executeRunStep(context.Background(), hostEngine(t), req, io.Discard, newTestTiConfig("p", "s"), nil)
	require.NoError(t, err)
	assert.Equal(t, preset, req.Envs["HARNESS_GIT_CLONE_METADATA_FILE"])
}

func TestStartStep_PollCarriesGitCloneMetadataWrittenToEnvPath(t *testing.T) {
	require.NoError(t, os.MkdirAll(pipeline.GetSharedVolPath(), 0o755))
	id := "gc-start-present"
	t.Cleanup(func() { _ = os.Remove(gitCloneReceiptPath(id)) })

	req := &api.StartStepRequest{
		StartStepRequestConfig: api.StartStepRequestConfig{
			ID:                        id,
			Kind:                      api.Run,
			Name:                      "clone",
			WorkingDir:                t.TempDir(),
			Envs:                      map[string]string{"RECEIPT": sampleGitCloneReceipt},
			ShowScriptInExecutionLogs: hideScript(),
			Run: api.RunConfig{
				Entrypoint: []string{"/bin/sh", "-c"},
				Command:    []string{`printf %s "$RECEIPT" > "$HARNESS_GIT_CLONE_METADATA_FILE"`},
			},
		},
	}

	resp := startAndPoll(t, req)
	assert.Equal(t, sampleGitCloneReceipt, resp.GitCloneMetadata)
}

func TestStartStep_PollLeavesGitCloneMetadataEmptyWhenFileMissing(t *testing.T) {
	id := "gc-start-missing"
	req := &api.StartStepRequest{
		StartStepRequestConfig: api.StartStepRequestConfig{
			ID:                        id,
			Kind:                      api.Run,
			Name:                      "clone",
			WorkingDir:                t.TempDir(),
			Envs:                      map[string]string{},
			ShowScriptInExecutionLogs: hideScript(),
			Run: api.RunConfig{
				Entrypoint: []string{"/bin/sh", "-c"},
				Command:    []string{"true"},
			},
		},
	}

	resp := startAndPoll(t, req)
	assert.Empty(t, resp.GitCloneMetadata)
}

func startAndPoll(t *testing.T, req *api.StartStepRequest) *api.PollStepResponse {
	t.Helper()
	exec := NewStepExecutor(hostEngine(t))
	require.NoError(t, exec.StartStep(context.Background(), req, nil, nopLogClient{}, newTestTiConfig("p", "s"), &api.LogConfig{}))

	done := make(chan struct{})
	var resp *api.PollStepResponse
	var err error
	go func() {
		resp, err = exec.PollStep(context.Background(), &api.PollStepRequest{ID: req.ID})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("PollStep timed out")
	}
	require.NoError(t, err)
	require.NotNil(t, resp)
	return resp
}
