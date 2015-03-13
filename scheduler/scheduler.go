// Copyright 2014 The Serviced Authors.
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
			close(cancel)
			s.threads.Wait()
			cancel = make(chan struct{})
			for _, manager := range managers {
				s.threads.Add(1)
				go func() {
					defer s.threads.Done()
					s.manage(cancel, conn, realm, manager)
				}()
			}
			realm = pool.Realm
		}

		select {
		case <-ev:
		case <-s.shutdown:
			return nil
		}
	}
}

func (s *Scheduler) manage(cancel <-chan struct, conn client.Connection, realm string, manager Manager) {
	leader := AddMaster(conn, s.hostID, realm, manager.Leader())
	defer leader.ReleaseLead()

	_cancel := make(chan struct{})
	defer close(cancel)

	done := make(chan struct{})

	for {
		ev, err := leader.TakeLead()
		if err != nil {
			glog.Errorf("Host %s could not become the leader (%s): %s", s.hostID, manager.Leader(), err)
			continue
		}

		select {
		case <-cancel:
			// did I shutdown before I became the leader?
			glog.Infof("Stopping manager for %s (%s)", s.hostID, manager.Leader())
			return
		default:
			// TODO: make sure the realm of the other managers coincides with this realm
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
		case <-cancel:
			return
		}
	}
}

func (s *Scheduler) Stop() {
	close(s.shutdown)
	s.threads.Wait()
}

type ResourceManager struct {
	threads sync.WaitGroup
	shutdown chan struct{}

	localConn client.Connection
	remoteConn client.Connection
}

type StorageManager struct {
}



func (s *scheduler) startResourceManager(cancel <-chan struct{}, realm string) {
	s.threads.Add(1)
	defer s.threads.Done()

	for {
		restart := func() bool {
			leader := zzk.NewResourceLeader(s.localConn, s.hostID, realm)
			ev, err := leader.TakeLead()
			if err != nil {
				glog.Errorf("Could not become resource leader: %s", err)
				return true
			}
			defer leader.ReleaseLead()

			// did I shutdown before I became the leader?
			select {
			case <-cancel:
				glog.Infof("Stopping resource manager for %s (%s)", s.hostID, realm)
				return false
			default:
				// make sure the realm of the storage leader coincides with the realm of the 
				// resource leader
				storageLeader, err := zzk.GetStorageLeader(s.localConn)
				if err != client.ErrNoLeaderFound && err != nil {
					glog.Errorf("Could not get the storage leader for %s (%s): %s", s.hostID, realm, err)
					return true
				} else if storagLeader.Realm != realm {
					// This master is obviously running in the wrong pool
					glog.Errorf("Invalid master pool (%s) for host %s", s.poolID, s.hostID)
					return true
				}
			}

			_cancel := make(chan struct{})
			defer close(_cancel)

			// start data synchronizers
			go s.syncRemote(_cancel)
			go s.syncLocal(_cancel)

			// start resource listeners
			s.startResourceListeners(_cancel)

			select {
			case <-ev:
				glog.Infof("Lost lead;  restarting resource manager for %s (%s)", s.hostID, realm)
				return true
			case <-cancel:
				glog.Infof("Stopping resource manager for %s (%s)", s.hostID, realm)
				return false
			}
		}()

		if restart {
			time.Sleep(time.Second)
		} else {
			return
		}
	}
}

func (s *scheduler) startResourceListeners(cancel <-chan struct{}) {
}

func (s *scheduler) startStorageManager(cancel <-chan struct{}, realm string) {
	s.threads.Add(1)
	defer s.threads.Done()

	leader := zzk.NewStorageLeader(s.localConn, s.hostID, realm)
	ev, err := leader.TakeLead()
	if err != nil {
		glog.Errorf("Could not become storage leader: %s", err)
		return
	}
	defer leader.ReleaseLead()

	select {
	case <-cancel:
		glog.Infof("Stopping storage manager for %s (%s)", s.hostID, realm)
		return
	default:
		// make sure the realm of the resource leader coincides with the realm of
		// the storage leader
		resourceLeader,err := zzk.GetResourceLeader(s.localConn)
		if err != client.ErrNoLeaderFound && err != nil {
			glog.Errorf("Could not get the resource  leader for %s (%s)", s.hostID, realm)
			return
		} else if resourceLeader.Realm != realm {
			// This master is obviously running in the wrong pool
			glog.Errorf("Invalid master pool (%s) for host %s", s.poolID, s.hostID)
			return
		}
	}

	// start storage listeners
	s.startStorageListeners(cancel)
}

func (s *scheduler) startStorageListeners(cancel <-chan struct{}) }
}



func (s *scheduler) Stop() {
	close(s.shutdown)
	s.threads.Wait()
}









type leaderFunc func(<-chan interface{}, coordclient.Connection, dao.ControlPlane, string, int)










type scheduler struct {
	sync.Mutex                     // only one process can stop and start the scheduler at a time
	cpDao         dao.ControlPlane // ControlPlane interface
	poolID        string           // pool where the master resides
	realm         string           // realm for which the scheduler will run
	instance_id   string           // unique id for this node instance
	shutdown      chan interface{} // Shuts down all the pools
	started       bool             // is the loop running
	zkleaderFunc  leaderFunc       // multiple implementations of leader function possible
	snapshotTTL   int
	facade        *facade.Facade
	stopped       chan interface{}
	registry      *registry.EndpointRegistry
	storageServer *storage.Server

	conn coordclient.Connection
}

// NewScheduler creates a new scheduler master
func NewScheduler(poolID string, instance_id string, storageServer *storage.Server, cpDao dao.ControlPlane, facade *facade.Facade, snapshotTTL int) (*scheduler, error) {
	s := &scheduler{
		cpDao:         cpDao,
		poolID:        poolID,
		instance_id:   instance_id,
		shutdown:      make(chan interface{}),
		stopped:       make(chan interface{}),
		zkleaderFunc:  Lead, // random scheduler implementation
		facade:        facade,
		snapshotTTL:   snapshotTTL,
		storageServer: storageServer,
	}
	return s, nil
}

// Start starts the scheduler
func (s *scheduler) Start() {
	s.Lock()
	defer s.Unlock()

	if s.started {
		return
	}
	s.started = true

	pool, err := s.facade.GetResourcePool(datastore.Get(), s.poolID)
	if err != nil {
		glog.Errorf("Could not acquire resource pool %s: %s", s.poolID, err)
		return
	}
	s.realm = pool.Realm

	go func() {

		defer func() {
			close(s.stopped)
			s.started = false
		}()

		for {
			var conn coordclient.Connection
			select {
			case conn = <-zzk.Connect("/", zzk.GetLocalConnection):
				if conn != nil {
					s.mainloop(conn)
				}
			case <-s.shutdown:
				return
			}

			select {
			case <-s.shutdown:
				return
			default:
				// restart
			}
		}

	}()
}

// mainloop acquires the leader lock and initializes the listener
func (s *scheduler) mainloop(conn coordclient.Connection) {
	// become the leader
	leader := zzk.NewHostLeader(conn, s.instance_id, s.realm, "/scheduler")
	event, err := leader.TakeLead()
	if err != nil {
		glog.Errorf("Could not become the leader: %s", err)
		return
	}
	defer leader.ReleaseLead()

	s.registry, err = registry.CreateEndpointRegistry(conn)
	if err != nil {
		glog.Errorf("Error initializing endpoint registry: %s", err)
		return
	}

	// did I shut down before I became the leader?
	select {
	case <-s.shutdown:
		return
	default:
	}

	var wg sync.WaitGroup
	defer wg.Wait()

	_shutdown := make(chan interface{})
	defer close(_shutdown)

	stopped := make(chan struct{}, 2)

	// monitor the resource pool
	monitor := zkservice.MonitorResourcePool(_shutdown, conn, s.poolID)

	// start the storage server
	wg.Add(1)
	go func() {
		defer glog.Infof("Stopping storage sync")
		defer wg.Done()
		if err := s.storageServer.Run(_shutdown, conn); err != nil {
			glog.Errorf("Could not maintain storage lead: %s", err)
			stopped <- struct{}{}
		}
	}()

	// synchronize with the remote
	wg.Add(1)
	go func() {
		defer glog.Infof("Stopping remote sync")
		defer wg.Done()
		s.remoteSync(_shutdown, conn)
	}()

	// synchronize locally
	wg.Add(1)
	go func() {
		defer glog.Infof("Stopping local sync")
		defer wg.Done()
		s.localSync(_shutdown, conn)
	}()

	wg.Add(1)
	go func() {
		defer glog.Infof("Stopping pool listeners")
		defer wg.Done()
		zzk.Start(_shutdown, conn, s, zkservice.NewServiceLockListener())
		stopped <- struct{}{}
	}()

	// wait for something to happen
	for {
		select {
		case <-event:
			glog.Warningf("Coup d'etat, re-electing...")
			return
		case <-stopped:
			glog.Warningf("Leader died, re-electing...")
			return
		case pool := <-monitor:
			if pool == nil || pool.Realm != s.realm {
				glog.Warningf("Realm changed, re-electing...")
				return
			}
		case <-s.shutdown:
			return
		}
	}
}

// Stop stops all scheduler processes for the master
func (s *scheduler) Stop() {
	s.Lock()
	defer s.Unlock()

	if !s.started {
		return
	}
	close(s.shutdown)
	<-s.stopped
}

// SetConnection implements zzk.Listener
func (s *scheduler) SetConnection(conn coordclient.Connection) { s.conn = conn }

// PostProcess implements zzk.Listener
func (s *scheduler) PostProcess(p map[string]struct{}) {}

// GetPath implements zzk.Listener
func (s *scheduler) GetPath(nodes ...string) string {
	return path.Join(append([]string{"/pools"}, nodes...)...)
}

// Ready implements zzk.Listener
func (s *scheduler) Ready() error {
	glog.Infof("Entering lead for realm %s!", s.realm)
	glog.Infof("Host Master successfully started")
	return nil
}

// Done implements zzk.Listener
func (s *scheduler) Done() {
	glog.Infof("Exiting lead for realm %s!", s.realm)
	glog.Infof("Host Master shutting down")
	return
}

// Spawn implements zzk.Listener
func (s *scheduler) Spawn(shutdown <-chan interface{}, poolID string) {

	// Get a pool-based connection
	var conn coordclient.Connection
	select {
	case conn = <-zzk.Connect(zzk.GeneratePoolPath(poolID), zzk.GetLocalConnection):
		if conn == nil {
			return
		}
	case <-shutdown:
		return
	}

	var cancel chan interface{}
	var done chan struct{}

	for {
		var node zkservice.PoolNode
		event, err := s.conn.GetW(zzk.GeneratePoolPath(poolID), &node)
		if err != nil {
			glog.Errorf("Error while monitoring pool %s: %s", poolID, err)
			return
		}

		if node.Realm == s.realm {
			if done == nil {
				cancel = make(chan interface{})
				done = make(chan struct{})

				go func() {
					defer close(done)
					s.zkleaderFunc(cancel, conn, s.cpDao, poolID, s.snapshotTTL)
				}()
			}
		} else {
			if done != nil {
				close(cancel)
				<-done
				done = nil
			}
		}

		select {
		case <-event:
		case <-done:
			return
		case <-shutdown:
			if done != nil {
				close(cancel)
				<-done
			}
			return
		}
	}
}
