// Copyright 2015 The Serviced Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package service

import (
	"fmt"

	"github.com/control-center/serviced/coordinator/client"
	"github.com/control-center/serviced/domain/host"
)

// GetHostsByPool returns the list of hosts for a specific pool
func GetHostsByPool(conn client.Connection, poolID string) ([]host.Host, error) {
	hostids, err := conn.Children(poolpath(poolID, hostpath()))
	if err != nil {
		return nil, err
	}
	hosts := make([]host.Host, len(hostids))
	for i, id := range hostids {
		var node HostNode
		if err := conn.Get(poolpath(poolID, hostpath(id)), &node); err != nil {
			return nil, err
		}
		hosts[i] = *node.Host
	}
	return hosts, nil
}

// HostSync is the zookeeper synchronization object for hosts
type HostSync struct {
	conn   client.Connection
	poolID string
	hosts  []host.Host
}

// NewHostSync creates a new HostSync object
func NewHostSync(conn client.Connection, poolID string, hosts []host.Host) HostSync {
	return HostSync{conn, poolID, hosts}
}

// DataMap transforms the list of hosts into a map[id]object
func (sync HostSync) DataMap() map[string]interface{} {
	datamap := make(map[string]interface{})
	for i, h := range sync.hosts {
		datamap[h.ID] = &sync.hosts[i]
	}
	return datamap
}

// IDMap returns the hostids as a hashmap
func (sync HostSync) IDMap() (map[string]struct{}, error) {
	hostids, err := sync.conn.Children(poolpath(sync.poolID, hostpath()))
	if err != nil {
		return nil, err
	}
	idmap := make(map[string]struct{})
	for _, id := range hostids {
		idmap[id] = struct{}{}
	}
	return idmap, nil
}

// Create creates a new host
func (sync HostSync) Create(data interface{}) error {
	path, host, err := sync.convert(data)
	if err != nil {
		return err
	}
	var node HostNode
	if err := sync.conn.Create(path, &node); err != nil {
		return err
	}
	node.Host = host
	return sync.conn.Set(path, &node)
}

// Update updates an existing host
func (sync HostSync) Update(data interface{}) error {
	path, host, err := sync.convert(data)
	if err != nil {
		return err
	}
	var node HostNode
	if err := sync.conn.Get(path, &node); err != nil {
		return err
	}
	node.Host = host
	return sync.conn.Set(path, &node)
}

// Delete removes an existing host
func (sync HostSync) Delete(id string) error {
	return sync.conn.Delete(poolpath(sync.poolID, hostpath(id)))
}

func (sync HostSync) convert(data interface{}) (string, *host.Host, error) {
	if host, ok := data.(*host.Host); ok {
		return poolpath(sync.poolID, hostpath(host.ID)), host, nil
	}

	return "", nil, fmt.Errorf("invalid type")
}
