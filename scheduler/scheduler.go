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
	"time"

	"github.com/control-center/serviced/coordinator/client"
	"github.com/control-center/serviced/zzk"
	"github.com/zenoss/glog"
)

type Manager interface {
	Run(cancel <-chan struct{}, hostID, realm string)
	Leader() string
}

type Scheduler struct {
	shutdown chan struct{}
	threads  sync.WaitGroup
	realmID  string
	hostID   string
	managers []Manager
}

func StartScheduler(realmID, hostID string, managers ...Manager) *Scheduler {
	scheduler := &Scheduler{
		shutdown: make(chan struct{}),
		threads:  sync.WaitGroup{},
		realmID:  realmID,
		hostID:   hostID,
		managers: managers,
	}

	go func() {
		for {
			var conn client.Connection
			select {
			case conn = <-zzk.Connect("/", zzk.GetLocalConnection):
				if conn != nil {
					scheduler.start(conn)
					scheduler.threads.Wait()
				}
			case <-scheduler.shutdown:
				return
			}

			select {
			case <-scheduler.shutdown:
				return
			default:
			}
		}
	}()

	return scheduler
}

func (s *Scheduler) Stop() {
	close(s.shutdown)
	s.threads.Wait()
}

func (s *Scheduler) start(conn client.Connection) error {
	for i := range s.managers {
		s.threads.Add(1)
		go func() {
			defer s.threads.Done()
			s.manage(conn, s.managers[i])
		}()
	}
}

func (s *Scheduler) manage(conn client.Connection, manager Manager) {
	leader := AddLeader(conn, s.hostID, s.realmID, manager.Leader())

	for {
		done := make(chan struct{}, 2)

		// become the leader
		ready := make(chan struct{})
		go func() {
			ev, err := leader.TakeLead()

			// send ready signal or quit
			select {
			case ready <- err:
				if err != nil {
					return
				}
			case <-s.shutdown:
				leader.ReleaseLead()
				return
			}

			// wait for exit
			<-ev
			done <- struct{}{}
		}()

		// waiting for leader to be ready
		select {
		case err := <-ready:
			if err != nil {
				glog.Errorf("Host %s could not become the leader (%s): %s", s.hostID, manager.Leader(), err)
				return
			}
		case <-s.shutdown:
			// Did I shutdown before I became the leader?
			glog.Infof("Stopping manager for %s (%s)", s.hostID, manager.Leader())
			return
		}

		// start the manager
		cancel := make(chan struct{})
		go func() {
			if err := manager.Run(cancel, s.hostID, realm); err != nil {
				glog.Warningf("Manager for host %s exiting: %s", s.hostID, err)
				time.Sleep(5 * time.Second)
			}
			done <- struct{}{}
		}()

		// waiting for something to happen
		select {
		case <-done:
			glog.Warningf("Host %s exited unexpectedly, reconnecting", s.hostID)
			close(cancel)
			leader.ReleaseLead()
			<-done
		case <-s.shutdown:
			glog.Infof("Host %s receieved signal to shutdown", s.hostID)
			close(cancel)
			leader.ReleaseLead()
			return
		}
	}
}
