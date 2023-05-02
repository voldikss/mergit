package main

import (
	log "github.com/sirupsen/logrus"
	"github.com/xanzy/go-gitlab"
	"time"
)

func main() {
	projects := getEffectiveProjects()

	log.Infoln("start ticker ...")
	ticker := time.NewTicker(time.Duration(config.PollIntervalS) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		for _, p := range projects {
			go func(p *gitlab.Project) {
				processProjectMergeRequests(p)
			}(p)
		}
	}
}
