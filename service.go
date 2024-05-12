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

	switch cmd {
	case "install":
		if err := s.Install(); err != nil {
			Logger.Error(fmt.Sprintf("service is already installed: %s", err.Error()))
		}
		if err := s.Start(); err != nil {
			Logger.Error(fmt.Sprintf("error while starting the service: %s", err.Error()))
		}
		return
	case "status":
		st, err := s.Status()
		if err != nil {
			Logger.Error(fmt.Sprintf("error while fetching service status: %s", err.Error()))
		}
		fmt.Println(st)
	case "stop":
		if err := s.Stop(); err != nil {
			Logger.Error(fmt.Sprintf("error while stopping the service: %s", err.Error()))
		}
	case "start":
		if err := s.Start(); err != nil {
			Logger.Error(fmt.Sprintf("error while restarting the service: %s", err.Error()))
		}
	case "uninstall":
		if err := s.Uninstall(); err != nil {
			Logger.Error(fmt.Sprintf("error while uninstalling the service: %s", err.Error()))
		}
	default:
		if err := s.Run(); err != nil {
			Logger.Error(fmt.Sprintf("error while running the service: %s", err.Error()))
			return
		}
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
