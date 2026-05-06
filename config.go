package main

import (
	"os"

	"gopkg.in/yaml.v3"
)

type IMAPIDConfig struct {
	Name         string `yaml:"name"`
	Version      string `yaml:"version"`
	Vendor       string `yaml:"vendor"`
	SupportEmail string `yaml:"support-email"`
}

type Config struct {
	Listen              string       `yaml:"listen"`
	Upstream            string       `yaml:"upstream"`
	UpstreamTLSName     string       `yaml:"upstream_tls_server_name"`
	LogLevel            string       `yaml:"log_level"`
	IMAPID              IMAPIDConfig `yaml:"imap_id"`
}

func defaultConfig() Config {
	return Config{
		Listen:          "127.0.0.1:1993",
		Upstream:        "imap.163.com:993",
		UpstreamTLSName: "imap.163.com",
		LogLevel:        "info",
		IMAPID: IMAPIDConfig{
			Name:         "Foxmail",
			Version:      "7.2.25.230",
			Vendor:       "Tencent",
			SupportEmail: "support@foxmail.com",
		},
	}
}

func loadConfig(path string) (Config, error) {
	cfg := defaultConfig()
	if path == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	return cfg, yaml.Unmarshal(data, &cfg)
}
