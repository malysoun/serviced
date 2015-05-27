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
	"sync"

	coordclient "github.com/control-center/serviced/coordinator/client"
	"github.com/control-center/serviced/coordinator/storage"
	"github.com/control-center/serviced/dao"
	"github.com/control-center/serviced/datastore"
	"github.com/control-center/serviced/facade"
	"github.com/control-center/serviced/zzk"
	"github.com/control-center/serviced/zzk/registry"
	zkservice "github.com/control-center/serviced/zzk/service"
	"github.com/zenoss/glog"

	"path"
)


type Manager interface{
	Run(cancel <-chan struct{}, hostID, realm string)
	Leader() string
}

type Scheduler struct {
	shutdown chan struct{}
	threads sync.WaitGroup
	poolID string
	hostID string
	managers []Manager
}

func StartScheduler(poolID, hostID string, managers ...Manager) *Scheduler {
	scheduler := &Scheduler{
		shutdown: make(chan struct{}),
		threads: sync.WaitGroup{},
		poolID: poolID,
		hostID: hostID,
		managers: managers,
	}

	go func() {
		for {
			select {
			case conn := <-zzk.Connect("/", zzk.GetLocalConnection):
				if conn != nil {
					if err := s.start(conn); err != nil {
						// prevent this method from spinning
						time.Sleep(5 * time.Second)
						glog.Warningf("Restarting master %s for pool %s", hostID, poolID)
					} else {
						return
					}
				}
			case <-s.shutdown:
				return
			}
		}
	}()

	return scheduler
}

func (s *Scheduler) start(conn client.Connection) error {
	cancel := make(chan struct{})
	defer close(cancel)

	realm := ""
	for {
		// Find the resource pool where this master is running
		var pool pool.ResourcePool
		ev, err := conn.GetW(path.Join("/pools", s.poolID), &zkservice.PoolNode{ResourcePool: &pool})
		if err != nil {
			glog.Errorf("Could not look up pool %s for master %s: %s", s.poolID, s.hostID, err)
			return err
		}

		if realm != pool.Realm {
			realm = pool.Realm
			close(cancel)
			s.threads.Wait()
			cancel = make(chan struct{})
			for i := range s.managers {
				s.threads.Add(1)
				go func() {
					defer s.threads.Done()
					s.manage(cancel, conn, realm, s.managers[i])
				}()
			}
		}

		select {
		case <-ev:
		case <-s.shutdown:
			return nil
		}
	}
}

func (s *Scheduler) manage(cancel <-chan struct, conn client.Connection, realm string, manager Manager) {
	var _cancel chan struct{}

	leader := AddMaster(conn, s.hostID, realm, manager.Leader())
	done := make(chan struct{})

	for {
		_cancel = make(chan struct{})

		// TODO: need to make sure all masters using the same zk is using the 
		// same pool
		ev, err := leader.TakeLead()
		select {
		case <-cancel:
			// did I shutdown before I became the leader?
			glog.Infof("Stopping manager for %s (%s)", s.hostID, manager.Leader())
			return
		default:
			// TODO: make sure the realm of the other managers coincides with this realm
			if err != nil {
				glog.Errorf("Host %s could not become the leader (%s): %s", s.hostID, manager.Leader(), err)
				continue
			}
		}

		go func() {
			if err := manager.Run(_cancel, s.hostID, realm); err != nil {
				glog.Warningf("Manager for host %s exiting: %s", s.hostID, err)
				time.Sleep(5 * time.Second)
				done <- struct{}{}
				return
			}
		}()

		select {
		case <-done:
			glog.Warningf("Host %s exited unexpectedly, reconnecting", s.hostID)
			leader.ReleaseLead()
		case <-ev:
			glog.Warningf("Host %s lost lead, reconnecting", s.hostID)
			close(_cancel)
			select {
			case <-done:
				case <-time.After(
			<-done
		case <-cancel:
			leader.ReleaseLead()
			close(_cancel)
			return
		}
	}
}

func (s *Scheduler) Stop() {
	close(s.shutdown)
	s.threads.Wait()
}
