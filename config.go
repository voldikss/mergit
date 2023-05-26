package main

import (
	"flag"
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

	LoggingLevel string
}

var config Config

func init() {
	var configFilePath string
	flag.StringVar(&configFilePath, "config", "config.yaml", "config file absolute path")
	flag.Parse()

	data, err := ioutil.ReadFile(configFilePath)
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

	if loggingLevel := os.Getenv("LOGGING_LEVEL"); len(loggingLevel) > 0 {
		switch loggingLevel {
		case "panic", "Panic", "PANIC":
			log.SetLevel(log.PanicLevel)
		case "fatal", "Fatal", "FATAL":
			log.SetLevel(log.FatalLevel)
		case "error", "Error", "ERROR":
			log.SetLevel(log.ErrorLevel)
		case "warn", "Warn", "WARN":
			log.SetLevel(log.WarnLevel)
		case "info", "Info", "INFO":
			log.SetLevel(log.InfoLevel)
		case "debug", "Debug", "DEBUG":
			log.SetLevel(log.DebugLevel)
		case "trace", "Trace", "TRACE":
			log.SetLevel(log.TraceLevel)
		default:
			log.Panicln("unknown logging level:", loggingLevel)
		}
	}
}
