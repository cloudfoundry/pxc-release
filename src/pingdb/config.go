package main

import (
	"errors"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	Timeout time.Duration `env:"TIMEOUT,required"`
	Port    string        `env:"PORT,required"`
}

func ParseConfig(cfg *Config) error {
	if err := env.Parse(cfg); err != nil {
		return err
	}

	if cfg.Timeout <= 0 {
		return errors.New("TIMEOUT must be a positive duration")
	}

	return nil
}
