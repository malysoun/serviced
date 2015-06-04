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
	"fmt"
	"path"
	"strconv"
	"time"

	"github.com/control-center/serviced/coordinator/client"
	"github.com/control-center/serviced/coordinator/storage"
	"github.com/control-center/serviced/domain/host"
	"github.com/zenoss/glog"
)

type StorageManager struct {
	conn    client.Connection
	monitor *storage.Monitor
	driver  storage.StorageDriver
	host    *host.Host
}

func (m *StorageManager) Leader() string {
	return "/managers/storage"
}

func (m *StorageManager) Run(cancel <-chan struct{}) error {
	glog.Infof("Processing storage manager duties")

	node := &storage.Node{
		Host:       *s.host,
		ExportPath: fmt.Sprintf("%s:%s", s.host.IPAddr, s.driver.ExportPath()),
		ExportTime: strconv.FormatInt(time.Now().UnixNano(), 16),
	}

	if exists, _ := m.conn.Exists("/storage/clients"); !exists {
		if err := m.conn.CreateDir("/storage/clients"); err != nil {
			glog.Errorf("Could not build storage clients: %s", err)
			return err
		}
	}

	// monitor dfs; log warnings each cycle; restart dfs if needed
	go m.monitor.MonitorDFSVolume(path.Join("/exports", m.driver.ExportPath()), s.host.IPAddr, node.ExportTime, cancel, s.monitor.DFSVolumeMonitorPollUpdateFunc)

	for {
		clients, ev, err := s.conn.ChildrenW("/storage/clients")
		if err != nil {
			glog.Errorf("Could not watch storage clients: %s", err)
			return err
		}
		s.driver.SetClients(clients...)
		if err := s.driver.Sync(); err != nil {
			glog.Errorf("Could not sync driver: %s", err)
			return err
		}
		select {
		case e := <-ev:
			glog.V(1).Infof("storage,server: receieved event: %s", e)
		case <-cancel:
			return nil
		}
	}
}