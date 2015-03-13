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
	"github.com/control-center/serviced/coordinator/client"
	"github.com/control-center/serviced/domain/pool"
	"github.com/zenoss/glog"
)

// GetResourcePools returns all of the available resource pools
func GetResourcePools(conn client.Connection) ([]pool.ResourcePool, error) {
	poolids, err := conn.Children(poolpath())
	if err != nil {
		return nil, err
	}
	pools := make([]pool.ResourcePool, len(poolids))
	for i, id := range poolids {
		var node PoolNode
		if err := conn.Get(poolpath(id), &node); err != nil {
			return nil, err
		}
		pools[i] = *node.ResourcePool
	}
	return pools, nil
}

// PoolSync is the zookeeper synchronization object for pools
type PoolSync struct {
	conn  client.Connection
	pools []pool.ResourcePool
}

// NewPoolSync creates a new PoolSync object
func NewPoolSync(conn client.Connection, pools []pool.ResourcePool) PoolSync {
	return PoolSync{conn, pools}
}

// DataMap transforms the list of resource pools into a map[id]object
func (sync PoolSync) DataMap() map[string]interface{} {
	datamap := make(map[string]interface{})
	for i, p := range sync.pools {
		datamap[p.ID] = &sync.pools[i]
	}
	return datamap
}

// IDMap returns the poolids as a hashmap
func (sync PoolSync) IDMap() (map[string]struct{}, error) {
	poolIDs, err := sync.conn.Children(poolpath())
	if err != nil {
		glog.Errorf("Could not get children of %s: %s", poolpath(), err)
		return nil, err
	}
	idmap := make(map[string]struct{})
	for _, id := range poolIDs {
		idmap[id] = struct{}{}
	}
	return idmap, nil
}

// Create creates a new resource pool
func (sync PoolSync) Create(data interface{}) error {
	return AddResourcePool(sync.conn, data.(*pool.ResourcePool))
}

// Update updates an existing resource pool
func (sync PoolSync) Update(data interface{}) error {
	return UpdateResourcePool(sync.conn, data.(*pool.ResourcePool))
}

// Delete removes an existing resource pool
func (sync PoolSync) Delete(id string) error {
	return RemoveResourcePool(sync.conn, id)
}
