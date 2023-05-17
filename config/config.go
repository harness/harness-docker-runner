// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package config

import (
	"github.com/kelseyhightower/envconfig"
)

// Config provides the system configuration.
type Config struct {
	Debug      bool   `envconfig:"DRONE_DEBUG"`
	Trace      bool   `envconfig:"DRONE_TRACE"`
	ServerName string `envconfig:"SERVER_NAME" default:"drone"`

	Runner struct {
		Volumes []string `envconfig:"CI_MOUNT_VOLUMES"`
	}

	Server struct {
		Bind              string `envconfig:"HTTPS_BIND" default:":3000"`
		CertFile          string `envconfig:"SERVER_CERT_FILE" default:"/tmp/certs/server-cert.pem"` // Server certificate PEM file
		KeyFile           string `envconfig:"SERVER_KEY_FILE" default:"/tmp/certs/server-key.pem"`   // Server key PEM file
		CACertFile        string `envconfig:"CLIENT_CERT_FILE" default:"/tmp/certs/ca-cert.pem"`     // CA certificate file
		SkipPrepareServer bool   `envconfig:"SKIP_PREPARE_SERVER" default:"false"`                   // skip prepare server, install docker / git
		Insecure          bool   `envconfig:"SERVER_INSECURE" default:"true"`                        // run in insecure mode
	}

	Client struct {
		Bind       string `envconfig:"HTTPS_BIND" default:":3000"`
		CertFile   string `envconfig:"CLIENT_CERT_FILE" default:"/tmp/certs/server-cert.pem"` // Server certificate PEM file
		KeyFile    string `envconfig:"CLIENT_KEY_FILE" default:"/tmp/certs/server-key.pem"`   // Server Key PEM file
		CaCertFile string `envconfig:"CA_CERT_FILE" default:"/tmp/certs/ca-cert.pem"`         // CA certificate file
		Insecure   bool   `envconfig:"CLIENT_INSECURE" default:"true"`                        // don't check server certificate
	}

	DelegateCapacity struct {
		ManagerEndpoint string `envconfig:"MANAGER_ENDPOINT"`
		Secret          string `envconfig:"DELEGATE_SECRET"`
		AccountID       string `envconfig:"ACCOUNT_ID"`
		MaxBuilds       int    `envconfig:"DELEGATE_CAPACITY"`
	}
}

// Load loads the configuration from the environment.
func Load() (Config, error) {
	cfg := Config{}
	err := envconfig.Process("", &cfg)
	conf = &cfg
	return cfg, err
}

// Load loads the configuration from the environment.
func GetConfig() *Config {
	return conf
}

var conf *Config
