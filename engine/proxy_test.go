package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProxyEnvsFromEnviron(t *testing.T) {
	tests := []struct {
		name    string
		environ []string
		want    map[string]string
	}{
		{
			name:    "no proxy variables",
			environ: []string{"PATH=/usr/bin", "CUSTOM_VAR=hello"},
			want:    map[string]string{},
		},
		{
			name:    "uppercase only is written under both names",
			environ: []string{"HTTP_PROXY=http://proxy:3128"},
			want:    map[string]string{"HTTP_PROXY": "http://proxy:3128", "http_proxy": "http://proxy:3128"},
		},
		{
			name:    "lowercase only is written under both names",
			environ: []string{"https_proxy=http://proxy:3128"},
			want:    map[string]string{"HTTPS_PROXY": "http://proxy:3128", "https_proxy": "http://proxy:3128"},
		},
		{
			name:    "uppercase wins when both are set",
			environ: []string{"http_proxy=http://lower:1", "HTTP_PROXY=http://upper:2"},
			want:    map[string]string{"HTTP_PROXY": "http://upper:2", "http_proxy": "http://upper:2"},
		},
		{
			name:    "mixed case name, as windows may store it",
			environ: []string{"Http_Proxy=http://proxy:3128"},
			want:    map[string]string{"HTTP_PROXY": "http://proxy:3128", "http_proxy": "http://proxy:3128"},
		},
		{
			name:    "empty value is ignored",
			environ: []string{"HTTP_PROXY=", "http_proxy=http://lower:1"},
			want:    map[string]string{"HTTP_PROXY": "http://lower:1", "http_proxy": "http://lower:1"},
		},
		{
			name:    "no_proxy alone propagates",
			environ: []string{"NO_PROXY=localhost,.corp"},
			want:    map[string]string{"NO_PROXY": "localhost,.corp", "no_proxy": "localhost,.corp"},
		},
		{
			name:    "value containing an equals sign is kept whole",
			environ: []string{"HTTPS_PROXY=http://proxy:3128/?a=b"},
			want:    map[string]string{"HTTPS_PROXY": "http://proxy:3128/?a=b", "https_proxy": "http://proxy:3128/?a=b"},
		},
		{
			name:    "similar names are not proxy variables",
			environ: []string{"HTTP_PROXY_USER=bob", "PROXY_HOST=proxy", "ALL_PROXY=socks5://proxy:1080"},
			want:    map[string]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, proxyEnvsFromEnviron(tc.environ))
		})
	}
}

func TestIsProxyEnv(t *testing.T) {
	for _, name := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "NO_PROXY", "no_proxy", "Http_Proxy"} {
		assert.True(t, isProxyEnv(name), name)
	}
	for _, name := range []string{"PATH", "PROXY_HOST", "HTTP_PROXY_USER", "ALL_PROXY", ""} {
		assert.False(t, isProxyEnv(name), name)
	}
}

func TestStepEnvs(t *testing.T) {
	hostProxy := proxyEnvsFromEnviron([]string{
		"HTTP_PROXY=http://host:3128",
		"HTTPS_PROXY=http://host:3128",
		"NO_PROXY=localhost",
	})
	environ := []string{
		"PATH=/usr/bin",
		"CUSTOM_VAR=hello-host",
		// left in the process environment by an earlier stage's setup
		"HTTP_PROXY=http://earlier-stage:1",
		"http_proxy=http://earlier-stage:1",
	}

	tests := []struct {
		name       string
		isHostStep bool
		stageEnvs  map[string]string
		envs       map[string]string
		proxyEnvs  map[string]string
		want       map[string]string
	}{
		{
			name:       "container step gets the host proxy and nothing else from the host",
			isHostStep: false,
			proxyEnvs:  hostProxy,
			want: map[string]string{
				"HTTP_PROXY": "http://host:3128", "http_proxy": "http://host:3128",
				"HTTPS_PROXY": "http://host:3128", "https_proxy": "http://host:3128",
				"NO_PROXY": "localhost", "no_proxy": "localhost",
			},
		},
		{
			name:       "host step gets the host environment and the startup proxy, not the earlier stage's",
			isHostStep: true,
			proxyEnvs:  hostProxy,
			want: map[string]string{
				"PATH": "/usr/bin", "CUSTOM_VAR": "hello-host",
				"HTTP_PROXY": "http://host:3128", "http_proxy": "http://host:3128",
				"HTTPS_PROXY": "http://host:3128", "https_proxy": "http://host:3128",
				"NO_PROXY": "localhost", "no_proxy": "localhost",
			},
		},
		{
			name:       "stage proxy wins over the host proxy for that variable only",
			isHostStep: false,
			stageEnvs:  map[string]string{"HTTP_PROXY": "http://stage:2"},
			proxyEnvs:  hostProxy,
			want: map[string]string{
				"HTTP_PROXY":  "http://stage:2",
				"HTTPS_PROXY": "http://host:3128", "https_proxy": "http://host:3128",
				"NO_PROXY": "localhost", "no_proxy": "localhost",
			},
		},
		{
			name:       "step proxy wins over the stage proxy",
			isHostStep: false,
			stageEnvs:  map[string]string{"HTTP_PROXY": "http://stage:2"},
			envs:       map[string]string{"HTTP_PROXY": "http://step:3"},
			proxyEnvs:  hostProxy,
			want: map[string]string{
				"HTTP_PROXY":  "http://step:3",
				"HTTPS_PROXY": "http://host:3128", "https_proxy": "http://host:3128",
				"NO_PROXY": "localhost", "no_proxy": "localhost",
			},
		},
		{
			name:       "lowercase pipeline proxy keeps the host value out of the uppercase name",
			isHostStep: false,
			envs:       map[string]string{"http_proxy": "http://step:3"},
			proxyEnvs:  hostProxy,
			want: map[string]string{
				"http_proxy":  "http://step:3",
				"HTTPS_PROXY": "http://host:3128", "https_proxy": "http://host:3128",
				"NO_PROXY": "localhost", "no_proxy": "localhost",
			},
		},
		{
			name:       "empty pipeline proxy hides the host proxy for that variable",
			isHostStep: false,
			stageEnvs:  map[string]string{"HTTP_PROXY": ""},
			proxyEnvs:  hostProxy,
			want: map[string]string{
				"HTTP_PROXY":  "",
				"HTTPS_PROXY": "http://host:3128", "https_proxy": "http://host:3128",
				"NO_PROXY": "localhost", "no_proxy": "localhost",
			},
		},
		{
			name:       "empty pipeline proxy hides the host proxy on a host step",
			isHostStep: true,
			stageEnvs:  map[string]string{"HTTP_PROXY": ""},
			proxyEnvs:  hostProxy,
			want: map[string]string{
				"PATH": "/usr/bin", "CUSTOM_VAR": "hello-host",
				"HTTP_PROXY":  "",
				"HTTPS_PROXY": "http://host:3128", "https_proxy": "http://host:3128",
				"NO_PROXY": "localhost", "no_proxy": "localhost",
			},
		},
		{
			name:       "empty lowercase pipeline proxy hides both spellings of the host proxy",
			isHostStep: true,
			envs:       map[string]string{"http_proxy": ""},
			proxyEnvs:  hostProxy,
			want: map[string]string{
				"PATH": "/usr/bin", "CUSTOM_VAR": "hello-host",
				"http_proxy":  "",
				"HTTPS_PROXY": "http://host:3128", "https_proxy": "http://host:3128",
				"NO_PROXY": "localhost", "no_proxy": "localhost",
			},
		},
		{
			name:       "non proxy stage and step envs are unchanged",
			isHostStep: false,
			stageEnvs:  map[string]string{"A": "stage", "B": "stage"},
			envs:       map[string]string{"B": "step"},
			want:       map[string]string{"A": "stage", "B": "step"},
		},
		{
			name:       "no host proxy leaves container steps as before",
			isHostStep: false,
			envs:       map[string]string{"A": "step"},
			want:       map[string]string{"A": "step"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, stepEnvs(tc.isHostStep, environ, tc.stageEnvs, tc.envs, tc.proxyEnvs))
		})
	}
}

func TestSetHostProxyEnvs(t *testing.T) {
	saved := hostProxyEnvs
	defer func() { hostProxyEnvs = saved }()

	SetHostProxyEnvs([]string{"HTTPS_PROXY=http://proxy:3128", "CUSTOM_VAR=hello"})
	assert.Equal(t, map[string]string{"HTTPS_PROXY": "http://proxy:3128", "https_proxy": "http://proxy:3128"}, hostProxyEnvs)
}
