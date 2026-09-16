package engine

import (
	"testing"

	"github.com/harness/harness-docker-runner/engine/spec"
	"github.com/stretchr/testify/assert"
)

const logResilienceFF = "CI_LOG_SERVICE_RESILIENCE"

func TestIsFeatureFlagEnabled(t *testing.T) {
	tests := []struct {
		name           string
		pipelineConfig *spec.PipelineConfig
		want           bool
	}{
		{
			name:           "nil pipeline config",
			pipelineConfig: nil,
			want:           false,
		},
		{
			name:           "nil envs",
			pipelineConfig: &spec.PipelineConfig{},
			want:           false,
		},
		{
			name:           "empty envs",
			pipelineConfig: &spec.PipelineConfig{Envs: map[string]string{}},
			want:           false,
		},
		{
			name:           "flag absent",
			pipelineConfig: &spec.PipelineConfig{Envs: map[string]string{"OTHER_FF": "true"}},
			want:           false,
		},
		{
			name:           "flag true",
			pipelineConfig: &spec.PipelineConfig{Envs: map[string]string{logResilienceFF: "true"}},
			want:           true,
		},
		{
			name:           "flag false",
			pipelineConfig: &spec.PipelineConfig{Envs: map[string]string{logResilienceFF: "false"}},
			want:           false,
		},
		{
			name:           "flag uppercase TRUE is not accepted",
			pipelineConfig: &spec.PipelineConfig{Envs: map[string]string{logResilienceFF: "TRUE"}},
			want:           false,
		},
		{
			name:           "flag numeric 1 is not accepted",
			pipelineConfig: &spec.PipelineConfig{Envs: map[string]string{logResilienceFF: "1"}},
			want:           false,
		},
		{
			name:           "flag empty string",
			pipelineConfig: &spec.PipelineConfig{Envs: map[string]string{logResilienceFF: ""}},
			want:           false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := &Engine{pipelineConfig: tc.pipelineConfig}
			assert.Equal(t, tc.want, e.IsFeatureFlagEnabled(logResilienceFF))
		})
	}
}

// A windows container only exposes a C drive, so a host path that follows
// WORKING_DIR onto another drive has to be rewritten before the container sees
// it. Otherwise docker refuses to create the container, reporting
// "hcs::CreateComputeSystem ... The parameter is incorrect." (CI-24226).
func TestToWindowsContainerDrive(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "non C drive is replaced by C",
			path: `D:\Temp\engine`,
			want: `C:\Temp\engine`,
		},
		{
			name: "clone workspace on a non C drive",
			path: `D:\Temp\harness-TL3MTDZqQ8C`,
			want: `C:\Temp\harness-TL3MTDZqQ8C`,
		},
		{
			name: "C drive is left alone",
			path: `C:\Temp\engine`,
			want: `C:\Temp\engine`,
		},
		{
			name: "lowercase drive letter",
			path: `e:\Temp\foo`,
			want: `C:\Temp\foo`,
		},
		{
			name: "forward slashes are converted",
			path: `D:\Temp\engine/abc-output.env`,
			want: `C:\Temp\engine\abc-output.env`,
		},
		{
			name: "path without a drive gets the C drive",
			path: "/addon",
			want: `C:\addon`,
		},
		{
			name: "docker socket is left alone",
			path: DockerSockWinPath,
			want: DockerSockWinPath,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, toWindowsContainerDrive(tc.path))
		})
	}
}

// toWindowsDrive keeps the drive letter it is given, which is what a path used
// on the host needs.
func TestToWindowsDriveKeepsExistingDrive(t *testing.T) {
	assert.Equal(t, `D:\Temp\engine`, toWindowsDrive(`D:\Temp\engine`))
	assert.Equal(t, `C:\addon`, toWindowsDrive("/addon"))
	assert.Equal(t, DockerSockWinPath, toWindowsDrive(DockerSockWinPath))
}
