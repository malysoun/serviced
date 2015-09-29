// Copyright 2015 The Serviced Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dfs

import "github.com/zenoss/glog"

func (dfs *DistributedFilesystem) DeleteSnapshot(snapshotID string) error {
	// Look up the snapshot
	snapshot, err := dfs.disk.Get(snapshotID)
	if err != nil {
		glog.Errorf("Could not find snapshot %s: %s", snapshotID, err)
		return err
	}
	vol, err := dfs.disk.Get(snapshot.Tenant())
	if err != nil {
		glog.Errorf("Could not get parent volume of snapshot %s: %s", snapshotID, err)
		return err
	}
	info, err := vol.SnapshotInfo(snapshotID)
	if err != nil {
		glog.Errorf("Could not get info for snapshot %s: %s", snapshotID, err)
		return err
	}
	// Delete the snapshot volume
	if err := vol.DeleteSnapshot(snapshotID); err != nil {
		glog.Errorf("Could not delete snapshot %s: %s", snapshotID, err)
		return err
	}
	// Remove the image from the registry
	dfs.cleanRegistrySnapshot(info.TenantID, info.Label)
	return nil
}

func (dfs *DistributedFilesystem) deleteRegistryTag(tenantID, tag string) {
	rImageMap, err := dfs.registry.SearchLibrary(library, tag)
	if err != nil {
		glog.Warningf("Could not search registry library %s for tag %s: %s", tenantID, tag, err)
		return
	}
	for rImage := range rImageMap {
		if err := dfs.docker.RemoveImage(rImage); err != nil {
			glog.Warningf("Could not remove image %s locally: %s", rImage, err)
		}
		if err := dfs.registry.DeleteImage(rImage); err != nil {
			glog.Warningf("Could not remove image %s from registry: %s", rImage, err)
		}
	}
}
