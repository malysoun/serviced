// Copyright 2014 The Serviced Authors.
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
	"fmt"
	"strings"

	zkservice "github.com/control-center/serviced/zzk/service"
	zkvirtualips "github.com/control-center/serviced/zzk/virtualips"
	"github.com/zenoss/glog"
)

type SyncError []error

func (errs SyncError) HasError() bool { return len(errs) }

func (errs SyncError) Error() string {
	if !errs.HasError() {
		return ""
	}

	errstr := make([]string, len(errs))
	for i, err := range errs {
		errstr[i] = err.Error()
	}
	return fmt.Sprintf("receieved %d errors: \n\t%s", len(errs), strings.Join(errstr, "\n\t"))
}

func (m *ResourceManager) syncLocal() error {
	var syncerr SyncError

	pools, err := m.f.GetResourcePools(m.ctx)
	if err != nil {
		glog.Errorf("Could not get resource pools: %s", err)
		return err
	} else if err := Synchronize(zkservice.NewPoolSync(m.lconn, pools)); err != nil {
		glog.Errorf("Could not synchronize resource pools: %s", err)
		syncerr = append(syncerr, err)
	}

	for _, pool := range pools {
		// update the virtual ips
		if err := Synchronize(zkvirtualips.NewVIPSync(m.lconn, pool.ID, pool.VirtualIPs)); err != nil {
			glog.Errorf("Could not synchronize virtual ips in pool %s: %s", pool.ID, err)
			syncerr = append(syncerr, err)
		}

		// update the hosts
		if hosts, err := m.f.FindHostsInPool(m.ctx, pool.ID); err != nil {
			glog.Errorf("Could not get hosts in pool %s: %s", pool.ID, err)
			syncerr = append(syncerr, err)
		} else if err := Synchronize(zkservice.NewHostSync(m.lconn, pool.ID, hosts)); err != nil {
			glog.Errorf("Could not synchronize hosts in pool %s: %s", pool.ID, err)
			syncerr = append(syncerr, err)
		}

		// update the services
		if services, err := m.f.GetServicesByPool(m.ctx, pool.ID); err != nil {
			glog.Errorf("Could not get services in pool %s: %s", pool.ID, err)
			syncerr = append(syncerr, err)
		} else if err := Synchronize(zkservice.NewServiceSync(m.lconn, pool.ID, services)); err != nil {
			glog.Errorf("Could not synchronize services in pool %s: %s", pool.ID, err)
			syncerr = append(syncerr, err)
		}
	}

	if syncerr.HasError() {
		return syncerr
	}

	return nil
}
