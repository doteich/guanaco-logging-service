package main

import (
	"fmt"

	"github.com/kardianos/service"
)

type programm struct {
	exit    chan struct{}
	service service.Service
}

func CreateService(id int, sName string, path string, cmd string) {

	sFN := fmt.Sprintf("%d_%s", id, sName)

	svcConfig := &service.Config{
		Name:        "guanaco_svc_" + sFN,
		DisplayName: "Guanaco Logging Service " + sFN,
		Description: "Guanaco Logging Service for " + sFN,
		Arguments:   []string{"-path", path},
	}

	prg := programm{exit: make(chan struct{})}

	s, err := service.New(&prg, svcConfig)

	if err != nil {
		Logger.Error(fmt.Sprintf("error while creating new service: %s", err))
	}

	prg.service = s

	if cmd == "install" {
		if err := s.Install(); err != nil {
			Logger.Error(fmt.Sprintf("service is already installed: %s", err.Error()))
		}
	}

	// if err := s.Run(); err != nil {
	// 	Logger.Error(fmt.Sprintf("error while starting the service: %s", err.Error()))
	// 	return
	// }
	if err := s.Start(); err != nil {
		Logger.Error(fmt.Sprintf("error while starting the service: %s", err.Error()))
	}

}

func (p *programm) Start(service.Service) error {

	go p.run()

	return nil
}

func (p *programm) Stop(service.Service) error {
	close(p.exit)
	return nil
}
