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

package scheduler

import (
	"time"

	"github.com/control-center/serviced/dao"
	"github.com/control-center/serviced/utils"
	"github.com/zenoss/glog"
)

// SnapshotTTL manages the time to live for snapshots
type SnapshotTTL struct {
	client dao.ControlPlane
	maxAge time.Duration
}

// NewSnapshotTTL creates a new snapshot ttl manager
func NewSnapshotTTL(client dao.ControlPlane, maxAge time.Duration) *SnapshotTTL {
	return &SnapshotTTL{client, maxAge}
}

// Leader returns the zk path to the leader
func (ttl *SnapshotTTL) Leader() string {
	return "/managers/ttl/snapshots"
}

// Run runs the ttl
func (ttl *SnapshotTTL) Run(cancel <-chan struct{}, realm string) error {
	glog.Infof("Starting snapshot ttl")
	utils.RunTTL(cancel, ttl, 10*time.Minute, ttl.maxAge)
	return nil
}

// Purge deletes snapshots based on age and rechecks based on the age of the
// oldest snapshot
func (ttl *SnapshotTTL) Purge(age time.Duration) (time.Duration, error) {
	expire := time.Now().Add(-age)

	var tenantIDs []string
	if err := ttl.client.GetTenantIDs("", &svcs); err != nil {
		glog.Errorf("Could not look up tenant services: %s", err)
		return 0, err
	}
	for _, tenantID := range tenantIDs {
		var ssinfo []dao.SnapshotInfo
		if err := ttl.client.ListSnapshots(tenantID, &ssinfo); err != nil {
			glog.Errorf("Could not list snapshots for tenantID %s: %s", tenantID, err)
			return 0, err
		}
		for _, si := range ssinfo {
			_, timestamp, err := dfs.ParseLabel(si.SnapshotID)
			if err != nil {
				continue
			}
			snapTime, err := time.Parse(dfs.TimeFormat, timestamp)
			if err != nil {
				continue
			}
			if timeToLive := expire.Sub(snapTime); timeToLive <= 0 {
				// snapshot has exceeded its expiration date
				if err := ttl.client.DeleteSnapshot(si.SnapshotID, nil); err != nil {
					glog.Errorf("Could not delete snapshot %s: %s", si.SnapshotID, err)
					return 0, err
				}
			} else if timeToLive < age {
				age = timeToLive
			}
		}
	}

	return age, nil
}