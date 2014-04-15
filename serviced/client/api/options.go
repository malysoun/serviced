package api

import (
	"fmt"
	"strings"

	"github.com/zenoss/serviced/dao"
)

// Handles URL data
type URL struct {
	Host string
	Port string
}

func (u *URL) Set(value string) error {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return fmt.Errorf("bad format: %s; must be formatted as HOST:PORT", value)
	}

	(*u).Host = parts[0]
	(*u).Port = parts[1]
	return nil
}

func (u *URL) String() string {
	return fmt.Sprintf("%s:%s", u.Host, u.Port)
}

// Mapping of docker image data
type ImageMap map[string]string

func (m *ImageMap) Set(value string) error {
	parts := strings.Split(value, ",")
	if len(parts) != 2 {
		return fmt.Errorf("bad format")
	}

	(*m)[parts[0]] = parts[1]
	return nil
}

func (m *ImageMap) String() string {
	var mapping []string
	for k, v := range m {
		mapping = append(mapping, k+","+v)
	}

	return strings.Join(mapping, " ")
}

// Mapping of mounted volumes to docker image ids
type VolumeMap map[string][2]string

func (m *VolumeMap) Set(value string) error {
	parts := strings.Split(value, ",")
	if len(parts) != 3 {
		return fmt.Errorf("bad format: %s; must be IMAGE_ID,HOST_PATH,DOCKER_PATH", value)
	}
	image := parts[0]
	host := parts[1]
	docker := parts[2]

	(*m)[image] = []string{host, docker}
	return nil
}

func (m *VolumeMap) String() string {
	var mapping []string
	for k, v := range m {
		mapping = append(mapping, fmt.Sprintf("%s,%s,%s", k, v[0], v[1]))
	}
	return strings.Join(mapping, " ")
}

// Mapping of port data
type PortMap map[string]dao.ServiceEndpoint

func (m *PortMap) Set(value string) error {
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return fmt.Errorf("bad format: %s; must be PROTOCOL:PORTNUM:PORTNAME", value)
	}
	protocol := parts[0]
	switch protocol {
	case "tcp", "udp":
	default:
		return fmt.Errorf("unsupported protocol: %s (udp|tcp)", protocol)
	}
	portnum, err := strconv.ParseUint(parts[1], 10, 16)
	if err != nil {
		return fmt.Errorf("invalid port number: %s", parts[1])
	}
	portname := parts[2]
	if portname == "" {
		return fmt.Errorf("port name cannot be empty")
	}
	port := protocol + ":" + portnum
	(*m)[port] = dao.ServiceEndpoint{Protocol: protocol, PortNumber: portnum, Application: portname}
	return nil
}

func (m *PortMap) String() string {
	var mapping []string
	for _, v := range m {
		mapping = append(mapping, fmt.Sprintf("%s:%s:%s", v.Protocol, v.PortNumber, v.Application))
	}
	return strings.Join(mapping, " ")
}