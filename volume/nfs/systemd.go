package nfs

import (
	"fmt"
	"os/exec"

	"github.com/control-center/serviced/utils"
)

type SystemdError struct {
	Output []byte
	Err    error
}

func (err *SystemdError) Error() string {
	return fmt.Sprintf("%s (%s)", err.Output, err.Err)
}

type SystemdService interface {
	Redhat() string
	Debian() string
}

type Service struct {
	redhat, debian string
}

func NewService(redhat, debian string) *Service {
	return &Service{redhat, debian}
}

func (s *Service) Redhat() string {
	return s.redhat
}

func (s *Service) Debian() string {
	return s.debian
}

type Systemd interface {
	Start() error
	Stop() error
	Reload() error
	Restart() error
}

func NewSystemdService(service SystemdService, platform int) Systemd {
	switch platform {
	case utils.Rhel:
		return &rhelsystemd{service}
	default:
		return &debsystemd{service}
	}
}

type rhelsystemd struct {
	service SystemdService
}

func (systemd *rhelsystemd) Start() error {
	cmd := exec.Command("systemctl", "reload-or-restart", systemd.service.Redhat())
	if output, err := cmd.CombinedOutput(); err != nil {
		return &SystemdError{output, err}
	}
	return nil
}

func (systemd *rhelsystemd) Stop() error {
	cmd := exec.Command("systemctl", "stop", systemd.service.Redhat())
	if output, err := cmd.CombinedOutput(); err != nil {
		return &SystemdError{output, err}
	}
	return nil
}

func (systemd *rhelsystemd) Reload() error {
	cmd := exec.Command("systemctl", "reload", systemd.service.Redhat())
	if output, err := cmd.CombinedOutput(); err != nil {
		return &SystemdError{output, err}
	}
	return nil
}

func (systemd *rhelsystemd) Restart() error {
	cmd := exec.Command("systemctl", "restart", systemd.service.Redhat())
	if output, err := cmd.CombinedOutput(); err != nil {
		return &SystemdError{output, err}
	}
	return nil
}

type debsystemd struct {
	service SystemdService
}

func (systemd *debsystemd) Start() error {
	cmd := exec.Command("/usr/bin/service", systemd.service.Debian(), "start")
	if output, err := cmd.CombinedOutput(); err != nil {
		return &SystemdError{output, err}
	}
	return nil
}

func (systemd *debsystemd) Stop() error {
	cmd := exec.Command("/usr/bin/service", systemd.service.Debian(), "stop")
	if output, err := cmd.CombinedOutput(); err != nil {
		return &SystemdError{output, err}
	}
	return nil
}

func (systemd *debsystemd) Reload() error {
	cmd := exec.Command("/usr/bin/service", systemd.service.Debian(), "reload")
	if output, err := cmd.CombinedOutput(); err != nil {
		return &SystemdError{output, err}
	}
	return nil
}

func (systemd *debsystemd) Restart() error {
	cmd := exec.Command("/usr/bin/service", systemd.service.Debian(), "restart")
	if output, err := cmd.CombinedOutput(); err != nil {
		return &SystemdError{output, err}
	}
	return nil
}
