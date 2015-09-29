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

// SnapshotInfo reports information about a particular snapshot
func (dfs *DistributedFilesystem) SnapshotInfo(snapshotID string) (*SnapshotInfo, error) {
	// get the snapshot volume
	vol, err := dfs.disk.Get(snapshotID)
	if err != nil {
		glog.Errorf("Could not find snapshot %s: %s", snapshotID, err)
		return nil, err
	}
	// get the tenant volume of the snapshot
	vol, err = dfs.disk.Get(vol.Tenant())
	if err != nil {
		glog.Errorf("Could not get parent volume for snapshot %s: %s", snapshotID, err)
		return nil, err
	}
	// get info about the snapshot
	snapshot, err := vol.SnapshotInfo(snapshotID)
	if err != nil {
		glog.Errorf("Could not get snapshot info for %s: %s", snapshotID, err)
		return err
	}
	return snapshot, err
}
