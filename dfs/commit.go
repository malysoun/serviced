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

	"github.com/zenoss/glog"
)

// Commit commits a container to the docker registry as a new layer of an
// application image.
func (dfs *DistributedFilesystem) Commit(dockerID, message string) (string, error) {
	// get the container
	ctr, err := dfs.docker.FindContainer(dockerID)
	if err != nil {
		glog.Errorf("Could not find container %s: %s", dockerID, err)
		return "", err
	}
	// do not commit if the container is running
	if ctr.State.Running {
		return "", errors.New("dfs: cannot commit a running container")
	}
	// check if the container is stale (ctr.Config.Image is the repotag)
	registryImage, err := dfs.registry.GetImage(ctr.Config.Image)
	if err != nil {
		glog.Errorf("Could not find image %s in registry: %s", ctr.Image, err)
		return "", err
	}
	// verify that we are committing to latest (ctr.Image is the UUID)
	if registryImage.Tag != docker.DockerLatest || registryImage.UUID != ctr.Image {
		return "", errors.New("dfs: cannot commit a stale container")
	}
	// commit the container
	image, err := dfs.docker.CommitContainer(ctr.ID, ctr.Config.Image)
	if err != nil {
		glog.Errorf("Could not commit container %s: %s", ctr.ID, err)
		return "", err
	}
	dfs.checkImageLayers(image.ID)
	if err := dfs.PushRegistryImage(image.Config.Image, image.ID); err != nil {
		glog.Errorf("Could not push image %s (%s): %s", image.Config.Image, image.ID, err)
		return "", err
	}
	return dfs.Snapshot(registryImage.Library, message)
}

// checkImageLayers warns if the number of image layers is approaching the max.
func (dfs *DistributedFilesystem) checkImageLayers(uuid string) {
	layers, err := dfs.docker.ImageHistory(uuid)
	if err != nil {
		glog.Warningf("Could not verify the number of image layers: %s", err)
		return
	}
	if layercount := len(layers); layercount > int(float64(docker.MaxImageLayers)*0.6) {
		glog.Warningf("Image %s has %d layers and is approaching the max layer count of %d.  Please squash image layers.", uuid, layercount, docker.MaxImageLayers)
		return
	}
}
