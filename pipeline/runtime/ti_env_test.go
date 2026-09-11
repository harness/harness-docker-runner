package runtime

import (
	b64 "encoding/base64"
	"testing"

	"github.com/harness/harness-docker-runner/engine/spec"
	tiCfg "github.com/harness/lite-engine/ti/config"
	"github.com/harness/ti-client/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testTIConfig() *tiCfg.Cfg {
	cfg := tiCfg.New(
		"https://ti.example.com",
		"raw-token",
		"acc", "org", "proj",
		"pipe", "build-1", "stage-1",
		"repo", "sha", "https://github.com/org/repo",
		"src", "tgt", "main",
		"", "",
		false, false, "", "",
	)
	return &cfg
}

func TestSetTiEnvVariables_NilConfig(t *testing.T) {
	step := &spec.Step{Name: "run", Envs: map[string]string{"KEEP": "yes"}}
	setTiEnvVariables(step, nil)
	assert.Equal(t, map[string]string{"KEEP": "yes"}, step.Envs)
}

func TestSetTiEnvVariables_ZeroCfgDoesNotPanic(t *testing.T) {
	step := &spec.Step{Name: "run"}
	setTiEnvVariables(step, &tiCfg.Cfg{})
	assert.Nil(t, step.Envs)
}

func TestSetTiEnvVariables_InjectsLiteEngineParity(t *testing.T) {
	step := &spec.Step{
		Name: "Test.Run",
		Envs: map[string]string{
			types.TiSvcEp: "stale",
			"KEEP":        "yes",
		},
	}
	setTiEnvVariables(step, testTIConfig())

	require.NotNil(t, step.Envs)
	assert.Equal(t, "yes", step.Envs["KEEP"])
	assert.Equal(t, "https://ti.example.com", step.Envs[types.TiSvcEp])
	assert.Equal(t, b64.StdEncoding.EncodeToString([]byte("raw-token")), step.Envs[types.TiSvcToken])
	assert.Equal(t, "acc", step.Envs[types.AccountIDEnv])
	assert.Equal(t, "org", step.Envs[types.OrgIDEnv])
	assert.Equal(t, "proj", step.Envs[types.ProjectIDEnv])
	assert.Equal(t, "pipe", step.Envs[types.PipelineIDEnv])
	assert.Equal(t, "stage-1", step.Envs[types.StageIDEnv])
	assert.Equal(t, "build-1", step.Envs[types.BuildIDEnv])
	assert.Equal(t, "Test.Run", step.Envs[types.StepIDEnv])
	assert.Equal(t, types.HarnessInfra, step.Envs[types.InfraEnv])
}

func TestSetTiEnvVariables_NilEnvsMap(t *testing.T) {
	step := &spec.Step{Name: "s"}
	setTiEnvVariables(step, testTIConfig())
	require.NotNil(t, step.Envs)
	assert.Equal(t, "https://ti.example.com", step.Envs[types.TiSvcEp])
}

func TestTiTokenSecrets(t *testing.T) {
	assert.Nil(t, TiTokenSecrets(nil))
	assert.Nil(t, TiTokenSecrets(&tiCfg.Cfg{}))

	secrets := TiTokenSecrets(testTIConfig())
	assert.Equal(t, []string{"raw-token", b64.StdEncoding.EncodeToString([]byte("raw-token"))}, secrets)
}
