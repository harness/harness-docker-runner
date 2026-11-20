// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/drone/runner-go/pipeline/runtime"
	"github.com/harness/harness-docker-runner/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleExplicitGitCloneReceipt = `{"repoUrl":"https://github.com/acme/repo.git","commitSha":"abc123","linesOfCode":42}`

func explicitGitCloneReceiptPath(stepID string) string {
	return fmt.Sprintf("%s/%s-explicit-git-clone-metadata.json", pipeline.GetSharedVolPath(), stepID)
}

// writeExplicitGitCloneReceipt writes a receipt under the shared volume and removes it afterwards.
func writeExplicitGitCloneReceipt(t *testing.T, stepID, content string) {
	t.Helper()
	path := explicitGitCloneReceiptPath(stepID)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	t.Cleanup(func() { _ = os.Remove(path) })
}

func TestReadExplicitGitCloneMetadata_FilePresent_ReturnsRawJSON(t *testing.T) {
	writeExplicitGitCloneReceipt(t, "egc-present", sampleExplicitGitCloneReceipt)
	assert.Equal(t, sampleExplicitGitCloneReceipt, readExplicitGitCloneMetadata("egc-present"))
}

func TestReadExplicitGitCloneMetadata_FileMissing_ReturnsEmpty(t *testing.T) {
	assert.Empty(t, readExplicitGitCloneMetadata("egc-does-not-exist"))
}

func TestReadExplicitGitCloneMetadata_EmptyFile_ReturnsEmpty(t *testing.T) {
	writeExplicitGitCloneReceipt(t, "egc-empty", "")
	assert.Empty(t, readExplicitGitCloneMetadata("egc-empty"))
}

func TestReadExplicitGitCloneMetadata_IsolatedPerStep(t *testing.T) {
	writeExplicitGitCloneReceipt(t, "egc-a", `{"commitSha":"aaa"}`)
	writeExplicitGitCloneReceipt(t, "egc-b", `{"commitSha":"bbb"}`)

	a := readExplicitGitCloneMetadata("egc-a")
	b := readExplicitGitCloneMetadata("egc-b")
	assert.Contains(t, a, "aaa")
	assert.NotContains(t, a, "bbb")
	assert.Contains(t, b, "bbb")
	assert.NotContains(t, b, "aaa")
}

func TestReadExplicitGitCloneMetadata_DoesNotParse(t *testing.T) {
	// The runner is a transport only; the manager parses. Malformed content must pass through.
	writeExplicitGitCloneReceipt(t, "egc-raw", "not json at all")
	assert.Equal(t, "not json at all", readExplicitGitCloneMetadata("egc-raw"))
}

func TestConvertStatus_CarriesExplicitGitCloneMetadata(t *testing.T) {
	resp := convertStatus(StepStatus{
		Status:                   Complete,
		State:                    &runtime.State{Exited: true, ExitCode: 0},
		ExplicitGitCloneMetadata: sampleExplicitGitCloneReceipt,
	})
	assert.Equal(t, sampleExplicitGitCloneReceipt, resp.ExplicitGitCloneMetadata)
}

func TestConvertStatus_NoMetadata_LeavesFieldEmpty(t *testing.T) {
	resp := convertStatus(StepStatus{Status: Complete, State: &runtime.State{Exited: true}})
	assert.Empty(t, resp.ExplicitGitCloneMetadata)
}

func TestConvertStatus_FailedStep_StillCarriesMetadata(t *testing.T) {
	// Current behaviour: forwarded regardless of exit code; the manager only consumes it on success.
	resp := convertStatus(StepStatus{
		Status:                   Complete,
		State:                    &runtime.State{Exited: true, ExitCode: 1},
		StepErr:                  fmt.Errorf("boom"),
		ExplicitGitCloneMetadata: sampleExplicitGitCloneReceipt,
	})
	assert.Equal(t, 255, resp.ExitCode)
	assert.Equal(t, sampleExplicitGitCloneReceipt, resp.ExplicitGitCloneMetadata)
}

func TestPollStepResponse_JSONKey_ExplicitGitCloneMetadata(t *testing.T) {
	// The delegate's ExecuteStepResponse binds to exactly this wire key.
	with, err := json.Marshal(convertStatus(StepStatus{ExplicitGitCloneMetadata: sampleExplicitGitCloneReceipt}))
	require.NoError(t, err)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(with, &m))
	assert.Equal(t, sampleExplicitGitCloneReceipt, m["explicit_git_clone_metadata"])

	without, err := json.Marshal(convertStatus(StepStatus{}))
	require.NoError(t, err)
	assert.NotContains(t, string(without), "explicit_git_clone_metadata")
}
