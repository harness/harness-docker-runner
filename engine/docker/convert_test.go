package docker

import (
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/harness/harness-docker-runner/engine/spec"
	"github.com/stretchr/testify/assert"
)

func TestToDeviceSlice(t *testing.T) {
	tests := []struct {
		name           string
		pipelineConfig *spec.PipelineConfig
		step           *spec.Step
		expected       []container.DeviceMapping
	}{
		{
			name:           "direct host_path device",
			pipelineConfig: &spec.PipelineConfig{},
			step: &spec.Step{
				Devices: []*spec.VolumeDevice{
					{HostPath: "/dev/kvm", DevicePath: "/dev/kvm"},
				},
			},
			expected: []container.DeviceMapping{
				{PathOnHost: "/dev/kvm", PathInContainer: "/dev/kvm", CgroupPermissions: "rwm"},
			},
		},
		{
			name:           "direct host_path with empty device_path defaults to host path",
			pipelineConfig: &spec.PipelineConfig{},
			step: &spec.Step{
				Devices: []*spec.VolumeDevice{
					{HostPath: "/dev/fuse"},
				},
			},
			expected: []container.DeviceMapping{
				{PathOnHost: "/dev/fuse", PathInContainer: "/dev/fuse", CgroupPermissions: "rwm"},
			},
		},
		{
			name: "legacy volume lookup path",
			pipelineConfig: &spec.PipelineConfig{
				Volumes: []*spec.Volume{
					{
						HostPath: &spec.VolumeHostPath{
							Name: "my-device",
							Path: "/dev/sda",
						},
					},
				},
			},
			step: &spec.Step{
				Devices: []*spec.VolumeDevice{
					{Name: "my-device", DevicePath: "/dev/sda"},
				},
			},
			expected: []container.DeviceMapping{
				{PathOnHost: "/dev/sda", PathInContainer: "/dev/sda", CgroupPermissions: "rwm"},
			},
		},
		{
			name: "legacy lookup skips non-device volume",
			pipelineConfig: &spec.PipelineConfig{
				Volumes: []*spec.Volume{
					{
						HostPath: &spec.VolumeHostPath{
							Name: "my-volume",
							Path: "/tmp/data",
						},
					},
				},
			},
			step: &spec.Step{
				Devices: []*spec.VolumeDevice{
					{Name: "my-volume", DevicePath: "/tmp/data"},
				},
			},
			expected: nil,
		},
		{
			name: "legacy lookup with unknown name skipped",
			pipelineConfig: &spec.PipelineConfig{
				Volumes: []*spec.Volume{},
			},
			step: &spec.Step{
				Devices: []*spec.VolumeDevice{
					{Name: "nonexistent", DevicePath: "/dev/foo"},
				},
			},
			expected: nil,
		},
		{
			name: "mixed direct and legacy devices",
			pipelineConfig: &spec.PipelineConfig{
				Volumes: []*spec.Volume{
					{
						HostPath: &spec.VolumeHostPath{
							Name: "my-device",
							Path: "/dev/sda",
						},
					},
				},
			},
			step: &spec.Step{
				Devices: []*spec.VolumeDevice{
					{HostPath: "/dev/kvm"},
					{Name: "my-device", DevicePath: "/dev/sda"},
				},
			},
			expected: []container.DeviceMapping{
				{PathOnHost: "/dev/kvm", PathInContainer: "/dev/kvm", CgroupPermissions: "rwm"},
				{PathOnHost: "/dev/sda", PathInContainer: "/dev/sda", CgroupPermissions: "rwm"},
			},
		},
		{
			name:           "empty devices list returns nil",
			pipelineConfig: &spec.PipelineConfig{},
			step: &spec.Step{
				Devices: []*spec.VolumeDevice{},
			},
			expected: nil,
		},
		{
			name: "host_path takes priority over name",
			pipelineConfig: &spec.PipelineConfig{
				Volumes: []*spec.Volume{
					{
						HostPath: &spec.VolumeHostPath{
							Name: "my-device",
							Path: "/dev/sda",
						},
					},
				},
			},
			step: &spec.Step{
				Devices: []*spec.VolumeDevice{
					{HostPath: "/dev/kvm", Name: "my-device", DevicePath: "/dev/sda"},
				},
			},
			expected: []container.DeviceMapping{
				{PathOnHost: "/dev/kvm", PathInContainer: "/dev/sda", CgroupPermissions: "rwm"},
			},
		},
		{
			name:           "multiple direct devices",
			pipelineConfig: &spec.PipelineConfig{},
			step: &spec.Step{
				Devices: []*spec.VolumeDevice{
					{HostPath: "/dev/kvm"},
					{HostPath: "/dev/fuse"},
					{HostPath: "/dev/net/tun"},
				},
			},
			expected: []container.DeviceMapping{
				{PathOnHost: "/dev/kvm", PathInContainer: "/dev/kvm", CgroupPermissions: "rwm"},
				{PathOnHost: "/dev/fuse", PathInContainer: "/dev/fuse", CgroupPermissions: "rwm"},
				{PathOnHost: "/dev/net/tun", PathInContainer: "/dev/net/tun", CgroupPermissions: "rwm"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := toDeviceSlice(tc.pipelineConfig, tc.step)
			assert.Equal(t, tc.expected, result)
		})
	}
}
