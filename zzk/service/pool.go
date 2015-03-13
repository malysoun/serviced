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
	"path"

	"github.com/control-center/serviced/coordinator/client"
	"github.com/control-center/serviced/domain/pool"
)

const (
	zkPool = "/pools"
)

func poolpath(nodes ...string) string {
	p := append([]string{zkPool}, nodes...)
	return path.Join(p...)
}

type PoolNode struct {
	*pool.ResourcePool
	version interface{}
}

// Version implements client.Node
func (node *PoolNode) Version() interface{} { return node.version }

// SetVersion implements client.Node
func (node *PoolNode) SetVersion(version interface{}) { node.version = version }

func AddResourcePool(conn client.Connection, pool *pool.ResourcePool) error {
	var node PoolNode
	if err := conn.Create(poolpath(pool.ID), &node); err != nil {
		return err
	}
	node.ResourcePool = pool
	return conn.Set(poolpath(pool.ID), &node)
}

func UpdateResourcePool(conn client.Connection, pool *pool.ResourcePool) error {
	var node PoolNode
	if err := conn.Get(poolpath(pool.ID), &node); err != nil {
		return err
	}
	node.ResourcePool = pool
	return conn.Set(poolpath(pool.ID), &node)
}

func RemoveResourcePool(conn client.Connection, poolID string) error {
	return conn.Delete(poolpath(poolID))
}
