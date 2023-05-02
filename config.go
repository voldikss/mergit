package main

import (
	"io/ioutil"
	"os"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

type Project struct {
	ID   int    `yaml:"id"`
	Path string `yaml:"path"`
}

type Config struct {
	GitLab struct {
		Url         string     `yaml:"url"`
		AccessToken string     `yaml:"access_token"`
		Projects    []*Project `yaml:"projects"`
	} `yaml:"gitlab"`

	PollIntervalS int `yaml:"poll_interval_s"`
}

var config Config

func init() {
	data, err := ioutil.ReadFile("config.yaml")
	if err != nil {
		log.Panicln("failed to read config.yaml file")
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		log.Panicln("failed to parse config")
	}

	if accessToken := os.Getenv("GITLAB_ACCESS_TOKEN"); len(accessToken) > 0 {
		config.GitLab.AccessToken = accessToken
	}
}
