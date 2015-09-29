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

import "github.com/zenoss/glog"

// PushRegistryImage writes an image to the docker registry and returns the image name.
func (dfs *DistributedFilesystem) PushRegistryImage(repotag string, uuid string) error {
	// parse the repo tag
	imageID, registryImage, err := dfs.registry.ParseImage(repotag)
	if err != nil {
		glog.Errorf("Could not parse image %s: %s", repotag, err)
		return err
	}
	// set the tag on the uuid
	if err := dfs.docker.TagImage(uuid, imageID); err != nil {
		glog.Errorf("Could not set tag %s on uuid %s: %s", imageID, uuid, err)
		return err
	}
	// save the tag to the registry
	registryImage.UUID = uuid
	if _, err := dfs.registry.SetImage(registryImage); err != nil {
		glog.Errorf("Could not update registry at %s: %s", imageID, err)
		return err
	}
	// push the image
	go func() {
		if err := dfs.docker.PushImage(imageID); err != nil {
			glog.Errorf("Could not push tag %s into registry: %s", repotag, err)
			return
		}
		if err := dfs.registry.UpdatePush(imageID); err != nil {
			glog.Errorf("Could not update tag %s in registry: %s", repotag, err)
			return
		}
	}()
	return nil
}
