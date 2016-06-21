package service

import (
	"path"
	"time"

	"github.com/control-center/serviced/coordinator/client"
	"github.com/control-center/serviced/domain/host"
	"github.com/zenoss/glog"
)

// HostOnlineMaster is a listener run by the master that monitors the availability of
// hosts within a pool by watching for children within the path
// /pools/POOLID/hosts/HOSTID/online
type HostOnlineMaster struct {
	conn   client.Connection
	poolID string
	online chan struct{}
}

func NewHostMonitor(poolid string) *HostOnlineMaster {
	return &HostOnlineMaster{
		poolID: poolid,
		online: make(chan struct{}),
	}
}

func (h *HostOnlineMaster) SetConnection(conn client.Connection) {
	h.conn = conn
}

func (h *HostOnlineMaster) GetPath(nodes ...string) string {
	return path.Join(append([]string{"/pools", h.poolID, "hosts"}, nodes...)...)
}

func (h *HostOnlineMaster) Ready() error { return nil }

func (h *HostOnlineMaster) Done() {}

func (h *HostOnlineMaster) PostProcess(p map[string]struct{}) {}

func (h *HostOnlineMaster) Spawn(cancel <-chan interface{}, hostID string) {

	// get the path of the node that tracks the connectivity of the host
	pth, err := h.waitStart(cancel, hostID)
	if err != nil {
		return
	} else if pth == "" {

		// the listener has been notified to shut down.
		return
	}

	// set up the connection timeout timer
	t := time.NewTimer(h.timeout())
	defer t.Stop()

	// let's keep track of outages
	outage := time.Now()
	isOnline := false

	// set up cancellable on coordinator events
	stop := make(chan struct{})
	defer close(stop)

	for {

		// check to see if the host is up
		ch, ev, err := h.conn.ChildrenW(pth, stop)
		if err == client.ErrNoNode {
			if count := h.removeInstancesOnHost(hostID); count > 0 {
				glog.Warningf("Host %s in pool %s shut down but had %d orphaned nodes", hostID, h.poolID, count)
			}
			glog.Infof("Host %s in pool %s has shut down", hostID, h.poolID)
			return
		} else if err != nil {
			glog.Errorf("Could not check online status for host %s in pool %s: %s", h.poolID, err)
			return
		}

		if len(ch) == 0 && isOnline {

			// host is dead, begin the countdown
			glog.V(2).Infof("Host %s in pool %s is not available", h.hostID, h.poolID)
			t.Reset(h.timeout())
			outage = time.Now()
			isOnline = false
		} else if len(ch) > 0 && !isOnline {

			// host is up, halt the countdown
			glog.V(0).Infof("Host %s in pool %s is available after %s", h.hostID, h.poolID, time.Since(outage))
			t.Stop()
			isOnline = true
		}

		if isOnline {
			// if the host is online, try to tell someone who cares
			select {
			case h.online <- struct{}{}:
			case <-ev:
			case <-cancel:
				return
			}
		} else {
			// wait for timeout or for host to go back online
			select {
			case <-t.C:
				hosts, err := GetCurrentHosts(h.conn, h.poolID)
				if err != nil {
					glog.Errorf("Could not look up active hosts in pool %s: %s", h.poolID, err)
					return
				}
				if len(hosts) > 0 || retries >= l.retries() {
					count := h.removeInstances(hostID)
					glog.Infof("Host %s in pool %s had a network interruption; %d nodes were rescheduled", hostID, h.poolID, count)
				} else {
					glog.V(2).Infof("No hosts to reschedule services; resetting timeout")
					t.Reset(h.timeout())
					retries++
				}
			case <-ev:
			case <-cancel:
				return
			}
		}
		close(stop)
		stop = make(chan struct{})
	}
}

// removeInstances cleans up all instances for a given host.
func (h *HostOnlineMaster) removeInstances(hostID string) int {
	hpth := h.GetPath(hostID, "instances")
	ch, err := h.conn.Children(hpth)
	if err != nil {
		glog.Errorf("Could not look up instances on host %s in pool %s: %s", hostID, h.poolID, err)
		return 0
	}
	for _, stateID := range ch {
		serviceID, instanceID := getServiceAndInstanceID(stateID)
		var hs HostState
		if err := conn.Get(hpth, &hs); err != nil {
			glog.Warningf("Could not look up instance %s on host %s: %s", stateID, hostID, err)
		} else {
			serviceID, instanceID = hs.ServiceID, hs.InstanceID
		}
		tx := conn.NewTransaction()
		tx.Delete(hpth)

		if serviceID != "" && instanceID != "" {
			spth := path.Join("/pools", h.poolID, "services", serviceID, instanceID)
			if ok, err := conn.Exists(spth); err != nil {
				glog.Warningf("Could not look up instance %s on service %s", instanceID, serviceID)
			} else if ok {
				tx.Delete(spth)
			}
		}
		if err := tx.Commit(); err != nil {
			glog.Warningf("Could not remove instance %s for service %s on host %s: %s", instanceID, serviceID, hostID)
		}
	}

	return len(ch)
}

// waitStart returns the path to monitor once it becomes available.
func (h *HostOnlineMaster) waitStart(cancel <-chan interface{}, hostID string) (string, error) {
	pth := h.GetPath(hostID, "online")

	// set up a cancellable on the event watcher
	stop := make(chan struct{})
	defer close(stop)

	// wait for the online node to exist
	for {
		ok, ev, err := h.conn.ExistsW(pth, stop)
		if err != nil {
			glog.Errorf("Could not monitor host %s in pool %s: %s", h.hostID, h.poolID, err)
			return false, err
		}

		// the node is ready, so let's move on
		if ok {
			return pth, nil
		}

		// waiting for the node to be ready
		select {
		case <-ev:
		case <-cancel:
			return "", nil
		}

		close(stop)
		stop = make(chan struct{})
	}
}

// timeout returns the amount of time to wait after the host is reported to be
// offline.
func (h *HostOnlineMaster) timeout() time.Duration {
	var p pool.PoolNode
	if err := h.conn.Get("/pools/"+h.poolID, &p); err != nil {
		glog.Warningf("Could not get pool connection timeout for %s: %s", h.poolID, err)
		return 0
	}
	return p.ConnectionTimeout
}

// GetRegisteredHosts returns a list of hosts that are currently active.  If
// there are zero active hosts, then it will wait until at least one host is
// available.
func (h *HostOnlineMaster) GetRegisteredHosts(cancel <-chan interface{}) ([]host.Host, error) {
	for {
		hosts, err := GetCurrentHosts(h.conn, h.poolID)
		if err != nil || len(hosts) > 0 {
			return hosts, err
		}
		glog.Infof("No hosts reported as active in pool %s, waiting", h.poolID)
		select {
		case <-h.online:
			glog.Infof("At least one host reported as active in pool %s, checking", h.poolID)
		case <-cancel:
			return nil, ErrShutdown
		}
	}
}

// GetCurrentHosts returns the list of hosts that are currently active.
func GetCurrentHosts(conn client.Connection, poolid string) ([]host.Host, error) {
	onlineHosts := make([]host.Host, 0)
	pth := path.Join("/pools", h.poolID, "hosts")
	ch, err := conn.Children(pth)
	if err != nil {
		return nil, err
	}
	for _, hostid := range ch {
		isOnline, err := IsHostOnline(conn, poolid, hostid)
		if err != nil {
			return nil, err
		}

		if isOnline {
			var hnode HostNode
			if err := conn.Get(path.Join(pth, hostid), &hnode); err != nil {
				return nil, err
			}
			onlineHosts = append(onlineHosts, *hnode.Host)
		}
	}
	return onlineHosts, nil
}

// IsHostOnline returns true if a provided host is currently active.
func IsHostOnline(conn client.Connection, poolid, hostid string) (bool, error) {
	pth := path.Join("/pools", poolid, "hosts", hostid, "online")
	ch, err := conn.Children(pth)
	if err != nil && err != client.ErrNoNode {
		return false, err
	}
	return len(ch) > 0, nil
}

// RegisterHost persists a registered host to the coordinator. This is managed
// by the worker node, so it is expected that the connection will be pre-loaded
// with the path to the resource pool.
func RegisterHost(cancel <-chan interface{}, conn client.Connection, hostID string) error {
	pth := path.Join("/hosts", hostID, "online")
	defer conn.Delete(pth)

	// set up cancellable on the event watcher
	stop := make(chan struct{})
	defer close(stop)

	for {

		// monitor the state of the host
		ch, ev, err := conn.ChildrenW(pth, stop)
		if err != nil {
			glog.Errorf("Could not verify the online status of host %s: %s", hostID, err)
			return err
		}

		// register the host if it isn't showing up as online.
		if len(ch) == 0 {
			err := conn.CreateIfExists(pth, &client.Dir{})
			if err != nil && err != client.ErrNodeExists {
				glog.Errorf("Could not register host %s as online: %s", hostID, err)
				return err
			}

			ehost, err = conn.CreateEphemeralIfExists(pth, &client.Dir{})
			if err != nil {
				glog.Errorf("Could not register host %s as active: %s", hostID, err)
				return err
			}
		}

		select {
		case <-ev:
		case <-cancel:
			glog.V(2).Infof("Host %s is shutting down", hostID)
			return nil
		}

		close(stop)
		stop = make(chan struct{})
	}
}
