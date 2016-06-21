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

package service

import (
	"github.com/control-center/serviced/coordinator/client"
	"github.com/control-center/serviced/domain/service"
	ss "github.com/control-center/serviced/domain/servicestate"
	"github.com/zenoss/glog"
)

// getServiceAndInstanceID parses a stateID in the form of serviceID-instanceID
func getServiceAndInstanceID(stateID string) (string, string) {
	parts := strings.SplitN(stateID, "-", 2)
	if len(parts) > 1 {
		return parts[0], parts[1]
	}
	return "", parts[0]
}

// addInstance creates a new service state and host instance
func addInstance(conn client.Connection, state ss.ServiceState) error {
	glog.V(2).Infof("Adding instance %+v", state)

	// check the object
	if err := state.ValidEntity(); err != nil {
		glog.Errorf("Could not validate service state %+v: %s", state, err)
		return err
	}

	// set up the host instances 
	if err := conn.CreateIfExists(

	// initialize the data to be written to the coordinator
	_, instanceID := getServiceAndInstanceID(state.ID)
	spath := servicepath(state.ServiceID, instanceID)
	snode := &ServiceStateNode{ServiceState: &state}
	hpath := hostpath(state.HostID, "instances", state.ID)
	hnode := NewHostState(&state)

	// build the transaction
	t := conn.NewTransaction()
	t.Create(spath, snode)
	t.Create(hpath, hnode)
	if err := t.Commit(); err != nil {
		glog.Errorf("Could not create service state nodes %s for service %s on host %s: %s", state.ID, state.ServiceID, state.HostID, err)
		return err
	}

	return nil
}

// removeInstance removes the service state and host instances
func removeInstance(conn client.Connection, serviceID, hostID, stateID string) error {
	glog.V(2).Infof("Removing instance %s", stateID)
	_, instanceID := getServiceAndInstanceID(stateID)
	t := conn.NewTransaction()

	// remove the service instance if found
	spath := servicepath(serviceID, instanceID)
	if ok, err := conn.Exists(spath); err != nil {
		glog.Errorf("Error checking path %s: %s", spath, err)
		return err
	} else if ok {
		t.Delete(spath)
	}

	// remove the host instance if found
	hpath := hostpath(hostID, "instances", stateID)
	if ok, err := conn.Exists(hpath); err != nil {
		glog.Errorf("Error checking path %s: %s", hpath, err)
		return err
	} else if ok {
		t.Delete(hpath)
	}

	// commit the transaction
	if err := t.Commit(); err != nil {
		glog.Errorf("Could not delete instance at %s and %s: %s", spath, hpath, err)
		return err
	}

	return nil
}

// updateInstance updates the service state and host instances
func updateInstance(conn client.Connection, hostID, stateID string, mutate func(*HostState, *ss.ServiceState)) error {
	glog.V(2).Infof("Updating instance %s", stateID)
	_, instanceID := getServiceAndInstanceID(stateID)

	// get the host state node
	hpath := hostpath(hostID, "instances", stateID)
	hdata := &HostState{}
	if err := conn.Get(hpath, hdata); err != nil {
		glog.Errorf("Could not get instance %s at host %s: %s", stateID, hostID, err)
		return err
	}

	// get the service state node
	spath := servicepath(hdata.ServiceID, instanceID)
	sdata := &ServiceStateNode{}
	if err := conn.Get(spath, sdata); err != nil {
		glog.Errorf("Could not get instance %s for service %s: %s", instanceID, hdata.ServiceID, err)
		return err
	}

	// transform the data and commit the changes transactionally
	mutate(hdata, sdata)
	if err := conn.NewTransaction().Set(hpath, hdata).Set(spath, sdata).Commit(); err != nil {
		glog.Errorf("Could not update instance %s on host %s: %s", stateID, hostID, err)
		return err
	}

	return nil
}

// removeInstancesOnHost removes all instances for a particular host. Will not
// delete if the instance cannot be found on the host (for when you have
// incongruent data).
func removeInstancesOnHost(conn client.Connection, hostID string) {
	instances, err := conn.Children(hostpath(hostID))
	if err != nil {
		glog.Errorf("Could not look up instances on host %s: %s", hostID, err)
		return
	}
	for _, stateID := range instances {
		var hs HostState
		if err := conn.Get(hostpath(hostID, stateID), &hs); err != nil {
			glog.Warningf("Could not look up host instance %s on host %s: %s", stateID, hostID, err)
		} else if err := removeInstance(conn, hs.ServiceID, hs.HostID, hs.ServiceStateID); err != nil {
			glog.Warningf("Could not remove host instance %s on host %s for service %s: %s", hs.ServiceStateID, hs.HostID, hs.ServiceID, err)
		} else {
			glog.V(2).Infof("Removed instance %s on host %s for service %s", hs.ServiceStateID, hs.HostID, hs.ServiceID, err)
		}
	}
}

// removeInstancesOnService removes all instances for a particular service. Will
// not delete if the instance cannot be found on the service (for when you have
// incongruent data).
func removeInstancesOnService(conn client.Connection, serviceID string) {
	instances, err := conn.Children(servicepath(serviceID))
	if err != nil {
		glog.Errorf("Could not look up instances on service %s: %s", serviceID, err)
		return
	}
	for _, stateID := range instances {
		var state ss.ServiceState
		if err := conn.Get(servicepath(serviceID, stateID), &ServiceStateNode{ServiceState: &state}); err != nil {
			glog.Warningf("Could not look up service instance %s for service %s: %s", stateID, serviceID, err)
		} else if err := removeInstance(conn, state.ServiceID, state.HostID, state.ID); err != nil {
			glog.Warningf("Could not remove service instance %s for service %s on host %s: %s", state.ID, state.ServiceID, state.HostID, err)
		} else {
			glog.V(2).Infof("Removed instance %s for service %s on host %s", state.ID, state.ServiceID, state.HostID, err)
		}
	}
}

// pauseInstance pauses a running host state instance
func pauseInstance(conn client.Connection, hostID, stateID string) error {
	return updateInstance(conn, hostID, stateID, func(hsdata *HostState, _ *ss.ServiceState) {
		if hsdata.DesiredState == int(service.SVCRun) {
			glog.V(2).Infof("Pausing service instance %s via host %s", stateID, hostID)
			hsdata.DesiredState = int(service.SVCPause)
		}
	})
}

// resumeInstance resumes a paused host state instance
func resumeInstance(conn client.Connection, hostID, stateID string) error {
	return updateInstance(conn, hostID, stateID, func(hsdata *HostState, _ *ss.ServiceState) {
		if hsdata.DesiredState == int(service.SVCPause) {
			glog.V(2).Infof("Resuming service instance %s via host %s", stateID, hostID)
			hsdata.DesiredState = int(service.SVCRun)
		}
	})
}

// UpdateServiceState does a full update of a service state
func UpdateServiceState(conn client.Connection, state *ss.ServiceState) error {
	if err := state.ValidEntity(); err != nil {
		glog.Errorf("Could not validate service state %+v: %s", state, err)
		return err
	}
	return updateInstance(conn, state.HostID, state.ID, func(_ *HostState, ssdata *ss.ServiceState) {
		*ssdata = *state
	})
}

// StopServiceInstance stops a host state instance
func StopServiceInstance(conn client.Connection, hostID, stateID string) error {
	// verify that the host is active
	var isActive bool
	hostIDs, err := GetActiveHosts(conn)
	if err != nil {
		glog.Warningf("Could not verify if host %s is active: %s", hostID, err)
		isActive = false
	} else {
		for _, hid := range hostIDs {
			if isActive = hid == hostID; isActive {
				break
			}
		}
	}
	if isActive {
		// try to stop the instance nicely
		return updateInstance(conn, hostID, stateID, func(hsdata *HostState, _ *ss.ServiceState) {
			glog.V(2).Infof("Stopping service instance via %s host %s", stateID, hostID)
			hsdata.DesiredState = int(service.SVCStop)
		})
	} else {
		// if the host isn't active, then remove the instance
		var hs HostState
		if err := conn.Get(hostpath(hostID, stateID), &hs); err != nil {
			glog.Errorf("Could not look up host instance %s on host %s: %s", stateID, hostID, err)
			return err
		}
		return removeInstance(conn, hs.ServiceID, hs.HostID, hs.ServiceStateID)
	}
}
