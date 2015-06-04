// Copyright 2015 The Serviced Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package scheduler

import (
	"fmt"
	"path"
	"sync"

	"github.com/control-center/serviced/commons"
	"github.com/control-center/serviced/coordinator/client"
	"github.com/control-center/serviced/dao"
	"github.com/control-center/serviced/domain/addressassignment"
	"github.com/control-center/serviced/domain/host"
	"github.com/control-center/serviced/domain/service"
	"github.com/control-center/serviced/zzk"
	zkservice "github.com/control-center/serviced/zzk/service"
	"github.com/control-center/serviced/zzk/virtualips"
	"github.com/zenoss/glog"
)

// ResourceManager manages the scheduling of services across hosts
type ResourceManager struct {
	cpclient dao.ControlPlane
	conn     client.Connection
}

// NewResourceManager returns a resource manager object
func NewResourceManager(cpclient dao.ControlPlane, conn client.Connection) *ResourceManager {
	return &ResourceManager{
		cpclient: cpclient,
		conn:     client.Connection,
	}
}

// Leader returns the zk path to the leader
func (m *ResourceManager) Leader() string {
	return "/managers/resource"
}

func (m *ResourceManager) Run(cancel <-chan struct{}) error {
	var wg sync.WaitGroup
	errch := make(chan error)

	wg.Add(1)
	go func() {
		defer wg.Done()
		zzk.Listen(cancel, errch, m.conn, &ResourceManager{cpclient: m.cpclient})
	}()

	if err := <-errch; err != nil {
		return err
	}
	wg.Wait()
	return nil
}

// ResourceListener manages resources per pool
type ResourceListener struct {
	conn     client.Connection
	cpclient dao.ControlClient
	registry *zkservice.HostRegistryListener
}

func (l *ResourceListener) SetConnection(conn client.Connection) {
	l.conn = conn
}

func (l *ResourceListener) GetPath(nodes ...string) string {
	return path.Join(append([]string{"/pools"}, nodes...))
}

func (l *ResourceListener) Ready() error { return nil }

func (l *ResourceListener) Done() {}

func (l *ResourceListener) Spawn(cancel <-chan interface{}, poolID string) {
	select {
	case conn := <-zzk.Connect(zzk.GetLocalConnection, l.GetPath(poolID)):
		if conn != nil {
			// set up the host registry
			if err := zkservice.InitHostRegistry(l.conn); err != nil {
				glog.Errorf("Could not initialize host registry for pool %s: %s", poolID, err)
				return
			}
			l.registry = zkservice.NewHostRegistryListener()
			// set up the service listener
			servicelistener := zkservice.NewServiceListener(l)
			// start the listeners
			zzk.Start(cancel, l.conn, servicelistener, l.registry)
		}
	case <-cancel:
	}
	return
}

// SelectHost chooses a host from the pool for the specified service. If the
// service has an address assignment, the host will already be selected.  If
// not, the host with the least amount of memory committed to running
// containers will be chosen.
func (l *ResourceListener) SelectHost(s *service.Service) (*host.Host, error) {
	glog.Infof("Looking for available hosts in pool %s", m.poolID)
	hosts, err := m.hostRegistry.GetHosts()
	if err != nil {
		glog.Errorf("Could not get available hosts for pool %s: %s", m.poolID, err)
		return nil, err
	}

	if address, err := getAddress(s); err != nil {
		glog.Errorf("Could not get address for service %s (%s): %s", s.Name, s.ID, err)
		return nil, err
	} else if address != nil {
		glog.Infof("Found address assignment for %s (%s) at %s, checking host availability", s.Name, s.ID, assignment.IPAddr)

		// Get the hostID from the address assignment
		hostID, err := getHostIDFromAddress(m.conn, address)
		if err != nil {
			glog.Errorf("Host not available at address %s: %s", address.IPAddr, err)
			return nil, err
		}

		// Check the host's availability
		for host := range hosts {
			if host.ID == hostID {
				return host, nil
			}
		}

		glog.Errorf("Host %s not available in pool %s.  Check to see if the host is running or reassign ips for service %s (%s)", hostID, l.poolID, s.Name, s.ID)
		return nil, fmt.Errorf("host %s not available in pool %s", hostID, l.poolID)
	}

	return NewServiceHostPolicy(s, m.cpclient).SelectHost(hosts)
}

// getAddress will extract the address assignment from the service
// if there is one available.
func getAddress(s *service.Service) (address *addressassignment.AddressAssignment, err error) {
	for i, ep := range s.Endpoints {
		if ep.IsConfigurable() {
			if ep.AddressAssignment.IPAddr != "" {
				address = &s.Endpoints[i].AddressAssignment
			} else {
				return nil, fmt.Errorf("missing address assignment")
			}
		}
	}
	return
}

// getHostIDFromAddress returns the hostID of the address assignment
func getHostIDFromAddress(conn client.Connection, address *addressassignment.AddressAssignment) (hostID string, err error) {
	if address.AssignmentType == commons.VIRTUAL {
		return virtualips.GetHostID(conn, address.IPAddr)
	}
	return address.HostID, nil
}
