package internal

import (
	"gopkg.in/yaml.v3"
	"os"
	"time"
)

const AppVersion = "0.0.1"

type Server struct {
	Id   string `yaml:"id"`
	Ip   string `yaml:"ip"`
	Port int    `yaml:"port"`
}

type Config struct {
	Version string

	ClusterID  string        `yaml:"cluster_id"`
	InstanceID string        `yaml:"instance_id"`
	Timeout    time.Duration `yaml:"timeout"`

	Servers []Server `yaml:"servers"`

	CheckScript   string        `yaml:"check_script"`
	CheckInterval time.Duration `yaml:"check_interval"`
	CheckRetries  int           `yaml:"check_retries"`
	CheckTimeout  time.Duration `yaml:"check_timeout"`

	RunScript  string        `yaml:"run_script"`
	RunTimeout time.Duration `yaml:"run_timeout"`

	StopScript  string        `yaml:"stop_script"`
	StopTimeout time.Duration `yaml:"stop_timeout"`
}

func LoadConfig(configPath string, cfg *Config) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(data, cfg)

	cfg.Version = AppVersion
	return err
}
