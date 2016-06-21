// Copyright 2014 The Serviced Authors.
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

package service

import (
	"errors"
	"path"

	"github.com/control-center/serviced/coordinator/client"
	"github.com/control-center/serviced/domain/host"
	"github.com/control-center/serviced/zzk"
	"github.com/zenoss/glog"
)

const (
	zkRegistry = "/registry"
)

var (
	ErrHostInvalid = errors.New("invalid host")
	ErrShutdown    = errors.New("listener shut down")
)

func hostregpath(nodes ...string) string {
	p := append([]string{zkRegistry, zkHost}, nodes...)
	return path.Clean(path.Join(p...))
}

// HostNode is the zk node for Host
type HostNode struct {
	*host.Host
	version interface{}
}

// ID implements zzk.Node
func (node *HostNode) GetID() string {
	return node.ID
}

// Create implements zzk.Node
func (node *HostNode) Create(conn client.Connection) error {
	return AddHost(conn, node.Host)
}

// Update implements zzk.Node
func (node *HostNode) Update(conn client.Connection) error {
	return UpdateHost(conn, node.Host)
}

// Version implements client.Node
func (node *HostNode) Version() interface{} {
	return node.version
}

// SetVersion implements client.Node
func (node *HostNode) SetVersion(version interface{}) {
	node.version = version
}

func SyncHosts(conn client.Connection, hosts []host.Host) error {
	nodes := make([]zzk.Node, len(hosts))
	for i := range hosts {
		nodes[i] = &HostNode{Host: &hosts[i]}
	}
	return zzk.Sync(conn, nodes, hostpath())
}

func AddHost(conn client.Connection, host *host.Host) error {
	var node HostNode
	if err := conn.Create(hostpath(host.ID), &node); err != nil {
		return err
	}
	node.Host = host
	return conn.Set(hostpath(host.ID), &node)
}

func UpdateHost(conn client.Connection, host *host.Host) error {
	var node HostNode
	if err := conn.Get(hostpath(host.ID), &node); err != nil {
		return err
	}
	node.Host = host
	return conn.Set(hostpath(host.ID), &node)
}

func RemoveHost(cancel <-chan interface{}, conn client.Connection, hostID string) error {
	if exists, err := zzk.PathExists(conn, hostpath(hostID)); err != nil {
		return err
	} else if !exists {
		return nil
	}

	// stop all the instances running on that host
	nodes, err := conn.Children(hostpath(hostID))
	if err != nil {
		return err
	}
	for _, stateID := range nodes {
		if err := StopServiceInstance(conn, hostID, stateID); err != nil {
			glog.Errorf("Could not stop service instance %s: %s", stateID, err)
			return err
		}
	}

	// wait until all the service instances have stopped
	done := make(chan struct{})
	defer func(channel *chan struct{}) { close(*channel) }(&done)
loop:
	for {
		nodes, event, err := conn.ChildrenW(hostpath(hostID), done)
		if err != nil {
			return err
		} else if len(nodes) == 0 {
			break
		}
		glog.V(2).Infof("%d services still running on host %s", len(nodes), hostID)

		select {
		case <-event:
			// pass
		case <-cancel:
			glog.Warningf("Giving up on waiting for services on host %s to stop", hostID)
			break loop
		}

		close(done)
		done = make(chan struct{})
	}

	// remove the parent node
	return conn.Delete(hostpath(hostID))
}
