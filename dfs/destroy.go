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
	"errors"

	"github.com/control-center/serviced/commons/docker"
	"github.com/control-center/serviced/domain/service"
	"github.com/zenoss/glog"
)

// Destroy destroys a volume and related images in the registry.
func (dfs *DistributedFilesystem) Destroy(tenantID string) error {
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
	// Get all the services
	svcs, err := dfs.data.GetServices(tenantID)
	if err != nil {
		glog.Errorf("Could not get services for tenant %s: %s", tenantID, err)
		return "", err
	}
	// Wait for services to stop, unless they are set to run, otherwise bail out
	serviceIDs := make([]string, len(svcs))
	for i, svc := range svcs {
		if svc.DesiredState != int(service.SVCStop) {
			return errors.New("dfs: found running services")
		}
		serviceIDs[i] = svc.ID
	}
	if err := dfs.data.WaitServices(service.SVCStop, dfs.timeout, serviceIDs); err != nil {
		glog.Errorf("Could not %s services for tenant %s: %s", service.SVCStop, tenantID, err)
		return "", err
	}
	// Stop sharing the tenant
	if err := dfs.net.DisableShare(tenantID); err != nil {
		glog.Errorf("Could not disable sharing for %s: %s", tenantID, err)
		return err
	}
	// Remove the share
	if err := dfs.net.RemoveShare(tenantID); err != nil {
		glog.Errorf("Could not remove share for %s: %s", tenantID, err)
		return err
	}
	// Delete the snapshots
	vol, err := dfs.disk.Get(tenantID)
	if err != nil {
		glog.Errorf("Could not get volume for tenant %s: %s", tenantID, err)
		return err
	}
	snapshots, err := vol.Snapshots()
	if err != nil {
		glog.Errorf("Could not get snapshots for volume %s: %s", tenantID, err)
		return err
	}
	for _, snapshot := range snapshots {
		if err := dfs.DeleteSnapshot(snapshot); err != nil {
			glog.Errorf("Could not delete snapshot %s: %s", snapshot, err)
			return err
		}
	}
	dfs.deleteRegistryTag(tenantID, docker.DockerLatest)
	// Remove the disk
	if err := dfs.disk.Remove(tenantID); err != nil {
		glog.Errorf("Could not remove volume %s: %s", tenantID, err)
		return err
	}
	return nil
}
