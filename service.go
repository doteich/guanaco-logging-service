package main

import (
	"fmt"

	"github.com/kardianos/service"
)

type programm struct {
	exit    chan struct{}
	service service.Service
}

func CreateService() {

	svcConfig := &service.Config{
		Name:        "guanaco-test",
		DisplayName: "guanaco-test-svc",
		Description: "Guanaco Logging Service",
	}

	prg := programm{exit: make(chan struct{})}

	s, err := service.New(&prg, svcConfig)

	if err != nil {
		Logger.Error(fmt.Sprintf("error while creating new service: %s", err))
	}

	prg.service = s

}

func (p *programm) Start(service.Service) error {

	return nil
}

func (p *programm) Stop(service.Service) error {
	close(p.exit)
	return nil
}
