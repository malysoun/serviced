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
	"github.com/control-center/serviced/domain/service"
)

// GetServicesByPool returns the list of services for a specific pool
func GetServicesByPool(conn client.Connection, poolID string) ([]service.Service, error) {
	svcids, err := conn.Children(poolpath(poolID, servicepath()))
	if err != nil {
		return nil, err
	}
	svcs := make([]service.Service, len(svcids))
	for i, id := range svcids {
		var node ServiceNode
		if err := conn.Get(poolpath(poolID, servicepath(id)), &node); err != nil {
			return nil, err
		}
		svcs[i] = *node.Service
	}
	return svcs, nil
}

// ServiceSync is the zookeeper synchronization object for services
type ServiceSync struct {
	conn     client.Connection
	poolID   string
	services []service.Service
}

// NewServiceSync creates a new ServiceSync object
func NewServiceSync(conn client.Connection, poolID string, services []service.Service) ServiceSync {
	return ServiceSync{conn, poolID, services}
}

// DataMap transforms the list of services into a map[id]object
func (sync ServiceSync) DataMap() map[string]interface{} {
	datamap := make(map[string]interface{})
	for i, s := range sync.services {
		datamap[s.ID] = &sync.services[i]
	}
	return datamap
}

// IDMap returns the serviceids as a hashmap
func (sync ServiceSync) IDMap() (map[string]struct{}, error) {
	ids, err := sync.conn.Children(poolpath(sync.poolID, servicepath()))
	if err != nil {
		return nil, err
	}
	idmap := make(map[string]struct{})
	for _, id := range ids {
		idmap[id] = struct{}{}
	}
	return idmap, nil
}

// Create creates a new service
func (sync ServiceSync) Create(data interface{}) error {
	path, svc, err := sync.convert(data)
	if err != nil {
		return err
	}
	var node ServiceNode
	if err := sync.conn.Create(path, &node); err != nil {
		return err
	}
	node.Service = svc
	return sync.conn.Set(path, &node)
}

// Update updates an existing service
func (sync ServiceSync) Update(data interface{}) error {
	path, svc, err := sync.convert(data)
	if err != nil {
		return err
	}
	var node ServiceNode
	if err := sync.conn.Get(path, &node); err != nil {
		return err
	}
	node.Service = svc
	return sync.conn.Set(path, &node)
}

// Delete removes an existing service
func (sync ServiceSync) Delete(id string) error {
	return sync.conn.Delete(poolpath(sync.poolID, servicepath(id)))
}

func (sync ServiceSync) convert(data interface{}) (string, *service.Service, error) {
	if svc, ok := data.(*service.Service); ok {
		return poolpath(sync.poolID, servicepath(svc.ID)), svc, nil
	}

	return "", nil, fmt.Errorf("invalid type")
}
