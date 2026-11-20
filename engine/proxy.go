// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package engine

import "strings"

// proxyEnvNames are the standard proxy variables. Each one is read and written
// under both its uppercase name and its lowercase name.
var proxyEnvNames = []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"}

// hostProxyEnvs holds the proxy variables of the runner host.
var hostProxyEnvs map[string]string

// SetHostProxyEnvs records the proxy variables found in environ, the runner
// process environment, so that every step receives them. It must be called at
// startup, before any stage is set up: stage setup writes the pipeline's proxy
// values into the process environment and never removes them.
func SetHostProxyEnvs(environ []string) {
	hostProxyEnvs = proxyEnvsFromEnviron(environ)
}

// proxyEnvsFromEnviron returns each proxy variable set in environ under both its
// uppercase and lowercase names. The uppercase value wins when both are set.
func proxyEnvsFromEnviron(environ []string) map[string]string {
	envs := map[string]string{}
	for _, name := range proxyEnvNames {
		var upper, other string
		for _, kv := range environ {
			k, v, ok := strings.Cut(kv, "=")
			if !ok || v == "" || !strings.EqualFold(k, name) {
				continue
			}
			if k == name {
				upper = v
			} else if other == "" {
				other = v
			}
		}
		value := upper
		if value == "" {
			value = other
		}
		if value != "" {
			envs[name] = value
			envs[strings.ToLower(name)] = value
		}
	}
	return envs
}

// isProxyEnv reports whether name is a proxy variable, in any case.
func isProxyEnv(name string) bool {
	for _, n := range proxyEnvNames {
		if strings.EqualFold(name, n) {
			return true
		}
	}
	return false
}

// fillProxyEnvs copies each proxy variable of proxyEnvs into envs, unless envs
// already sets that variable under any case, even to an empty value. An empty
// value is how a pipeline turns the host proxy off for that variable. Windows
// treats env names case-insensitively, so filling only the missing case would
// leave two different values for one variable.
func fillProxyEnvs(envs, proxyEnvs map[string]string) {
	for _, name := range proxyEnvNames {
		if hasProxyEnv(envs, name) {
			continue
		}
		for _, k := range []string{name, strings.ToLower(name)} {
			if v, ok := proxyEnvs[k]; ok {
				envs[k] = v
			}
		}
	}
}

// hasProxyEnv reports whether envs sets name, in any case. An empty value
// counts as set, so a pipeline can turn the host proxy off for that variable.
func hasProxyEnv(envs map[string]string, name string) bool {
	for k := range envs {
		if strings.EqualFold(k, name) {
			return true
		}
	}
	return false
}
