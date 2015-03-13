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

package virtualips

import (
	"fmt"
	"path"

	"github.com/control-center/serviced/coordinator/client"
	"github.com/control-center/serviced/domain/pool"
)

// VIPSync is the zookeeper synchronization object for virtual ips
type VIPSync struct {
	conn   client.Connection
	poolID string
	vips   []pool.VirtualIP
}

// NewVIPSync creates a new VIPSync object
func NewVIPSync(conn client.Connection, poolID string, vips []pool.VirtualIP) VIPSync {
	return VIPSync{conn, poolID, vips}
}

// DataMap transforms the list of virtual ips into a map[id]object
func (sync VIPSync) DataMap() map[string]interface{} {
	datamap := make(map[string]interface{})
	for i, vip := range sync.vips {
		datamap[vip.IP] = &sync.vips[i]
	}
	return datamap
}

// IDMap returns the virtual ip ids as a hashmap
func (sync VIPSync) IDMap() (map[string]struct{}, error) {
	ids, err := sync.conn.Children(sync.path(""))
	if err != nil {
		return nil, err
	}
	idmap := make(map[string]struct{})
	for _, id := range ids {
		idmap[id] = struct{}{}
	}
	return idmap, nil
}

// Create creates a new virtual ip
func (sync VIPSync) Create(data interface{}) error {
	path, vip, err := sync.convert(data)
	if err != nil {
		return err
	}
	var node VirtualIPNode
	if err := sync.conn.Create(path, &node); err != nil {
		return err
	}
	node.VirtualIP = vip
	return sync.conn.Set(path, &node)
}

// Update updates an existing virtual ip
func (sync VIPSync) Update(data interface{}) error {
	path, vip, err := sync.convert(data)
	if err != nil {
		return err
	}
	var node VirtualIPNode
	if err := sync.conn.Get(path, &node); err != nil {
		return err
	}
	node.VirtualIP = vip
	return sync.conn.Set(path, &node)
}

// Delete removes an existing virtual ip
func (sync VIPSync) Delete(id string) error {
	return sync.conn.Delete(sync.path(id))
}

func (sync VIPSync) path(id string) string {
	return path.Join("/pools", sync.poolID, vippath(id))
}

func (sync VIPSync) convert(data interface{}) (string, *pool.VirtualIP, error) {
	if vip, ok := data.(*pool.VirtualIP); ok {
		return sync.path(vip.IP), vip, nil
	}
	return "", nil, fmt.Errorf("invalid type")
}
