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
	"io"
	"time"

	"github.com/control-center/serviced/commons/docker"
	"github.com/control-center/serviced/domain/service"
	"github.com/zenoss/glog"
)

// Snapshot captures volume, data, and image information about an application
// at a specfic point in time.
func (dfs *DistributedFilesystem) Snapshot(tenantID, message string) (string, error) {
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
	// Get the tenant volume
	vol, err := dfs.disk.Get(tenantID)
	if err != nil {
		glog.Errorf("Could not get volume for tenant %s: %s", tenantID, err)
		return "", err
	}
	// Pause running services
	serviceIDs := make([]string, len(svcs))
	for i, svc := range svcs {
		if svc.DesiredState == int(service.SVCRun) {
			defer dfs.data.ScheduleService(svc.ID, false, service.SVCRun)
			if _, err := dfs.data.ScheduleService(svc.ID, false, service.SVCPause); err != nil {
				glog.Errorf("Could not %s service %s (%s): %s", service.SVCPause, svc.Name, svc.ID, err)
				return "", err
			}
		}
		serviceIDs[i] = svc.ID
	}
	if err := dfs.data.WaitServices(service.SVCPause, dfs.timeout, serviceIDs); err != nil {
		glog.Errorf("Could not %s services for tenant %s: %s", service.SVCPause, tenantID, err)
		return "", err
	}
	// Snapshot the image repository
	if err := dfs.updateRegistryTags(tenantID, docker.DockerLatest, label); err != nil {
		glog.Errorf("Could not snapshot repo %s: %s", tenantID, err)
		return "", err
	}
	// Take a snapshot
	label := time.Now().UTC().TimeFormat(timeFormat)
	if err := vol.Snapshot(label, message); err != nil {
		glog.Errorf("Could not snapshot volume for tenant %s: %s", tenantID, err)
		return "", err
	}
	// Write service metadata
	if exportFile, err := vol.WriteMetadata(label, "services.json"); err != nil {
		glog.Errorf("Could not write service metadata for snapshot %s of tenant %s: %s", label, tenantID, err)
		return "", err
	} else if err := exportMetadata(exportFile, svcs); err != nil {
		glog.Errorf("Could not export services to label %s of tenant %s: %s", label, tenantID, err)
		return "", err
	}
	return tenantID + "_" + label, nil
}

// updateRegistryTags updates the tags for an existing repository
func (dfs *DistributedFilesystem) updateRegistryTags(tenantID, oldLabel, newLabel string) error {
	// look up the current image info
	registryImages, err := dfs.registry.SearchLibraryByTag(tenantID, oldLabel)
	if err != nil {
		glog.Errorf("Could not get repos from library %s with tag %s: %s", tenantID, oldLabel, err)
		return err
	}
	// retag the image
	for _, registryImage := range registryImages {
		if _, err := dfs.PullImage(registryImage.String()); err != nil {
			glog.Errorf("Could not pull image %s: %s", registryImage, err)
			return err
		}
		registryImage.Tag = newLabel
		if err := dfs.PushImage(registryImage.String(), registryImage.UUID); err != nil {
			glog.Errorf("Could not push image %s (%s): %s", registryImage, registryImage.UUID, err)
			return err
		}
	}
	return nil
}

// export writes metadata to snapshots
func exportMetadata(writer io.WriteCloser, data interface{}) error {
	defer writer.Close()
	return json.NewEncoder(writer).Encode(data)
}
