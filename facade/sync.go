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

package facade

type PoolSync struct {
	f     Facade
	ctx   datastore.Context
	pools []pool.ResourcePool
}

func NewPoolSync(f Facade, pools []pool.ResourcePool) PoolSync {
	return PoolSync{f, datastore.Get(), pools}
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
	pools, err := sync.f.GetResourcePools(sync.ctx)
	if err != nil {
		glog.Errorf("Could not get resource pools: %s", err)
		return nil, err
	}
	idmap := make(map[string]struct{})
	for _, p := range pools {
		idmap[p.ID] = struct{}{}
	}
	return idmap, nil
}

type HostSync struct {
	f      Facade
	poolID string
	hosts  []host.Host
}

type ServiceSync struct {
	f        Facade
	poolID   string
	services []service.Service
}