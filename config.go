package proxy

import (
	"github.com/kelseyhightower/envconfig"
)

var c *Config

type Config struct {
	OriginDomain string `envconfig:"ORIGIN_DOMAIN"`
	OriginScheme string `envconfig:"ORIGIN_SCHEME" default:"http"`
}

func init() {
	load()
}

func load() *Config {
	var cnf Config
	if c != nil {
		return c
	}
	err := envconfig.Process("", &cnf)
	if err != nil {
		panic(err)
	}
	c = &cnf
	return c
}

func GetConfig() *Config {
	return c
}
