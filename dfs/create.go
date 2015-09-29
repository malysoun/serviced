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
	"github.com/control-center/serviced/commons"
	"github.com/control-center/serviced/commons/docker"
	"github.com/zenoss/glog"
)

// Create sets up the volume and the registry for its images.
func (dfs *DistributedFilesystem) Create(tenantID string) error {
	// get the services
	svcs, err := dfs.data.GetServices(tenantID)
	if err != nil {
		glog.Errorf("Could not get services of tenant %s: %s", tenantID, err)
		return err
	}
	// build the volume
	vol, err := dfs.disk.Create(tenantID)
	if err != nil {
		glog.Errorf("Could not initialize volume for tenant %s: %s", tenantID, err)
		return err
	}
	// add the volume to the share
	if err := dfs.net.AddShare(tenantID, vol.Path()); err != nil {
		glog.Errorf("Could not add share for volume %s: %s", tenantID, err)
		return err
	}
	// set up the registry
	images := make(map[string]string)
	for _, svc := range svcs {
		registryImageID, ok := images[svc.BaseImageID]
		if !ok {
			registryImageID, err = dfs.createRegistryImage(tenantID, svc.BaseImageID)
			if err != nil {
				glog.Errorf("Could not initialize image %s of tenant %s to the registry: %s", svc.BaseImageID, tenantID, err)
				return err
			}
			images[svc.BaseImageID] = registryImageID
		}
		svc.ImageID = registryImageID
		if err := dfs.data.UpdateService(svc); err != nil {
			glog.Errorf("Could not update image %s for service %s (%s): %s", svc.ImageID, svc.Name, svc.ID, err)
			return err
		}
	}
	// enable networking for tenant
	if err := dfs.net.EnableShare(tenantID); err != nil {
		glog.Errorf("Could not enable sharing for %s: %s", tenantID, err)
		return err
	}
	return nil
}

// createRegistryImage builds the repo for the tenant.
func (dfs *DistributedFilesystem) createRegistryImage(tenantID, imageID string) (string, error) {
	// Download the image if it isn't already there
	image, err := dfs.docker.FindImage(imageID)
	if err != nil {
		if docker.IsImageNotFound(err) {
			if img, err = dfs.docker.PullImage(image); err != nil {
				glog.Errorf("Could not pull image %s: %s", imageID, err)
				return "", err
			}
		} else {
			glog.Errorf("Could not look up image %s: %s", imageID, err)
			return "", err
		}
	}
	// Parse the repo
	registryImage := &commons.ImageID{
		User: tenantID,
		Repo: img.ImageID.Repo,
		Tag:  docker.DockerLatest,
	}
	// Push the image
	if err := dfs.PushImage(registryImage.String(), image.ID); err != nil {
		return "", err
	}
	return registryImage.String(), nil
}
