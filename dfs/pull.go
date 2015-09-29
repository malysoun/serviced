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
	"github.com/zenoss/glog"
)

// PullRegistryImage reads an image from the docker registry and returns the uuid of
// the image.
func (dfs *DistributedFilesystem) PullRegistryImage(repotag string) (string, error) {
	// find the image locally
	image, err := dfs.docker.FindImage(repotag)
	if err != nil && !docker.IsImageNotFound(err) {
		glog.Errorf("Could not look up image %s: %s", repotag, err)
		return "", err
	}
	// compare the image to what is in the registry
	imageName, registryImage, err := dfs.registry.GetImage(repotag)
	if err != nil {
		glog.Errorf("Could not find image %s in registry: %s", repotag, err)
		return "", err
	}
	// pull the image if it doesn't exist locally
	if registryImage == nil || registryImage.UUID != image.ID {
		if registryImage.LastPush.Unix() > 0 {
			if image, err = dfs.docker.PullImage(imageName); err != nil {
				glog.Errorf("Could not pull image %s: %s", repotag, err)
				return "", err
			} else if registryImage.UUID != image.ID {
				return "", errors.New("image not pushed")
			}
		}
	}
	return registryImage.UUID, nil
}
