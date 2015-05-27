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
package scheduler

import (
	"time"

	"github.com/control-center/serviced/coordinator/client"
	"github.com/control-center/serviced/domain/host"
	"github.com/control-center/serviced/domain/service"
	"github.com/control-center/serviced/utils"
	zkservice "github.com/control-center/serviced/zzk/service"
	zkvirtualips "github.com/control-center/serviced/zzk/virtualips"
	"github.com/zenoss/glog"
)

// LocalSync is the scheduler manager for local data synchronization
type LocalSync struct {
	cpclient dao.ControlClient
	conn     client.Connection
}

// NewLocalSync initializes a new local sync manager
func NewLocalSync(cpclient dao.ControlClient, conn client.Connection) *LocalSync {
	return &LocalSync{cpclient, conn}
}

// Leader is the path to the leader node
func (m *LocalSync) Leader() string {
	return "/managers/sync/local"
}

// Run starts the local sync manager
func (m *LocalSync) Run(cancel <-chan struct{}, realm string) {
	glog.Infof("Starting local synchronizer")
	utils.RunTTL(cancel, m, 30*time.Second, 3*time.Hour)
}

// Purge performs the synchronization
func (m *LocalSync) Purge(age time.Duration) (time.Duration, error) {
	var syncerr SyncError

	var pools []pool.ResourcePools
	if err := m.cpclient.GetResourcePools(dao.EntityRequest{}, &pools); err != nil {
		glog.Errorf("Could not get resource pools: %s", err)
		return err
	} else if Sync(zkservice.NewPoolSync(m.conn, pools)); err != nil {
		glog.Errorf("Could not synchronize resource pools: %s", err)
		syncerr = append(syncerr, err)
	}

	for _, pool := range pools {
		// update the virtual ips
		if err := Sync(zkvirtualips.NewVIPSync(m.conn, pool.ID, pool.VirtualIPs)); err != nil {
			glog.Errorf("Could not synchronize virtual ips in pool %s: %s", pool.ID, err)
			syncerr = append(syncerr, err)
		}

		// update the hosts
		var hosts []host.Host
		if err := m.cpclient.GetHosts(dao.HostRequest{PoolID: pool.ID}, &hosts); err != nil {
			glog.Errorf("Could not get hosts in pool %s: %s", pool.ID, err)
			syncerr = append(syncerr, err)
		} else if err := Sync(zkservice.NewHostSync(m.conn, pool.ID, hosts)); err != nil {
			glog.Errorf("Could not synchronize hosts in pool %s: %s", pool.ID, err)
			syncerr = append(syncerr, err)
		}

		// update the services
		var svcs []service.Service
		if err := m.cpclient.GetServices(dao.ServiceRequest{PoolID: pool.ID}, &svcs); err != nil {
			glog.Errorf("Could not get services in pool %s: %s", pool.ID, err)
			syncerr = append(syncerr, err)
		} else if err := Sync(zkservice.NewServiceSync(m.conn, pool.ID, &svcs)); err != nil {
			glog.Errorf("Could not synchronize services in pool %s: %s", pool.ID, err)
			syncerr = append(syncerr, err)
		}
	}

	if syncerr.HasError() {
		return 0, syncerr
	}

	return age, nil
}
