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
// Package agent implements a service that runs on a serviced node. It is
// responsible for ensuring that a particular node is running the correct services
// and reporting the state and health of those services back to the master
// serviced.

package dfs

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/control-center/serviced/commons/docker"
	"github.com/control-center/serviced/domain/service"
	"github.com/zenoss/glog"
)

// Rollback reverts a tenant's services, volume, and repos to the state of the snapshot
func (dfs *DistributedFilesystem) Rollback(snapshotID string, forceRestart bool) error {
	// Look up the snapshot
	snapshot, err := dfs.disk.Get(snapshotID)
	if err != nil {
		glog.Errorf("Could not find snapshot %s: %s", snapshotID, err)
		return err
	}
	tenantID := snapshot.TenantID()
	vol, err := dfs.disk.Get(tenantID)
	if err != nil {
		glog.Errorf("Could not get volume for tenant %s: %s", tenantID, err)
		return err
	}
	info, err := vol.SnapshotInfo(snapshotID)
	if err != nil {
		glog.Errorf("Could not get info about snapshot %s: %s", snapshotID, err)
		return err
	}
	label := info.Label
	// Prevent service scheduler from creating new instances
	lock := dfs.data.GetTenantLock(tenantID)
	if err != nil {
		glog.Errorf("Could not get lock for tenant %s: %s", tenantID, err)
		return "", err
	}
	if err := lock.Lock(); err != nil {
		glog.Errorf("Could not lock services for tenant %s: %s", tenantID, err)
		return "", err
	}
	defer lock.Unlock()
	// Get all the services and make sure that they are stopped
	svcs, err := dfs.data.GetServices(snapshot.Tenant())
	if err != nil {
		glog.Errorf("Could not get services for tenant %s: %s", tenantID, err)
		return err
	}
	serviceIDs := make([]string, len(svcs))
	for i, svc := range svcs {
		if svc.DesiredState != int(service.SVCStop) {
			if forceRestart {
				defer dfs.data.ScheduleService(svc.ID, false, service.DesiredState(svc.DesiredState))
				if _, err := dfs.data.ScheduleService(svc.ID, false, service.SVCStop); err != nil {
					glog.Errorf("Could not %s service %s (%s)", service.SVCStop, svc.Name, svc.ID, err)
					return err
				}
			} else {
				return errors.New("dfs: found running services")
			}
		}
		serviceIDs[i] = svc.ID
	}
	if err := dfs.data.WaitServices(service.SVCStop, dfs.timeout, serviceIDs); err != nil {
		glog.Errorf("Could not %s service for tenant %s: %s", service.SVCStop, tenantID, err)
		return err
	}
	// Rollback the services
	if importFile, err := vol.ReadMetadata(label, "services.json"); err != nil {
		glog.Errorf("Could not read service metadata of snapshot %s: %s", snapshotID, err)
		return err
	} else if err := importMetadata(importFile, &svcs); err != nil {
		glog.Errorf("Could not import services from snapshot %s: %s", snapshotID, err)
		return err
	}
	if err := dfs.data.RestoreServices(tenantID, svcs); err != nil {
		glog.Errorf("Could not restore services from snapshot %s: %s", snapshotID, err)
		return err
	}
	// Rollback the images
	if err := dfs.updateRegistryTags(tenantID, label, docker.DockerLatest); err != nil {
		glog.Errorf("Could not rollback repo %s: %s", tenantID, err)
		return err
	}
	// Stop sharing the tenant
	if err := dfs.net.DisableShare(tenantID); err != nil {
		glog.Errorf("Could not disable sharing for %s: %s", tenantID, err)
		return err
	}
	defer dfs.net.EnableShare(tenantID)
	// Rollback the volume
	if err := vol.Rollback(label); err != nil {
		glog.Errorf("Could not rollback snapshot %s for tenant %s: %s", tenantID, err)
		return err
	}
	return nil
}

func importMetadata(reader io.ReadCloser, data interface{}) error {
	defer reader.Close()
	return json.NewDecoder(reader).Decode(data)
}
