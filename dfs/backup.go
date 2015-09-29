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

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/zenoss/glog"
)

// Backup exports all application data into an export stream.
func (dfs *DistributedFilesystem) Backup(cancel <-chan struct{}, writer io.Writer) error {
	tarfile := tar.NewWriter(writer)
	defer tarfile.Close()
	// Get the service templates
	if templates, err := dfs.data.GetServiceTemplates(); err != nil {
		glog.Errorf("Could not get service templates for backup: %s", err)
		return err
	} else if err := encoder.Encode(templates); err != nil {
		glog.Errorf("Could not marshal service templates for backup: %s", err)
		return err
	} else if err := writeTar(tarfile, "DATA/templates", buffer); err != nil {
		glog.Errorf("Could not write service templates for backup: %s", err)
		return err
	}
	repotags, err := dfs.data.GetServiceTemplateImages()
	if err != nil {
		glog.Errorf("Could not get images for service templates: %s", err)
		return err
	}
	select {
	case <-cancel:
		return ErrCancelled
	default:
	}
	// Get the resource pools
	encoder := json.NewEncoder(buffer)
	if pools, err := dfs.data.GetResourcePools(); err != nil {
		glog.Errorf("Could not get resource pools for backup: %s", err)
		return err
	} else if err := encoder.Encode(pools); err != nil {
		glog.Errorf("Could not marshal resource pools for backup: %s", err)
		return err
	} else if err := writeTar(tarfile, "DATA/pools", buffer); err != nil {
		glog.Errorf("Could not write resource pools for backup: %s", err)
		return err
	}
	select {
	case <-cancel:
		return ErrCancelled
	default:
	}
	// Get the hosts
	if hosts, err := dfs.data.GetHosts(); err != nil {
		glog.Errorf("Could not get hosts for backup: %s", err)
		return err
	} else if err := encoder.Encode(hosts); err != nil {
		glog.Errorf("Could not marshal hosts for backup: %s", err)
		return err
	} else if err := writeTar(tarfile, "DATA/hosts", buffer); err != nil {
		glog.Errorf("Could not write hosts for backup: %s", err)
		return err
	}
	select {
	case <-cancel:
		return ErrCancelled
	default:
	}
	tenantIDs, err := dfs.data.GetTenantIDs()
	if err != nil {
		glog.Errorf("Could not get tenants for backup: %s", err)
		return err
	}
	// Snapshot each tenant
	for _, tenantID := range tenantIDs {
		vol, err := dfs.disk.Get(tenantID)
		if err != nil {
			glog.Errorf("Could not get volume for tenant %s: %s", tenantID, err)
			return err
		}
		snapshotID, err := dfs.Snapshot(tenantID, "")
		if err != nil {
			glog.Errorf("Could not snapshot %s: %s", tenantID, err)
			return err
		}
		defer dfs.DeleteSnapshot(snapshotID)
		select {
		case <-cancel:
			return ErrCancelled
		default:
		}
		snapshot, err := vol.SnapshotInfo(snapshotID)
		if err != nil {
			glog.Errorf("Could not get snapshot info for tenant %s: %s", tenantID, err)
			return err
		}
		// export the snapshot
		if err := vol.Export(snapshot.Label, "", buffer); err != nil {
			glog.Errorf("Could not export snapshot %s: %s", snapshotID, err)
			return err
		} else if err := writeTar(tarfile, fmt.Sprintf("APPS/%s/volume", tenantID), buffer); err != nil {
			glog.Errorf("Could not write snapshot for backup: %s", err)
			return err
		}
		select {
		case <-cancel:
			return ErrCancelled
		default:
		}
		// export the snapshot images
		rImages, err := dfs.registry.SearchLibrary(tenantID, snapshot.Label)
		if err != nil {
			glog.Errorf("Could not get registry images for %s at %s: %s", tenantID, snapshot.Label, err)
			return err
		} else if err := encoder.Encode(rImages); err != nil {
			glog.Errorf("Could not marshal registry images for %s: %s", tenantID, err)
			return err
		} else if err := writeTar(tarfile, fmt.Sprintf("APPS/%s/registry", tenantID), buffer); err != nil {
			glog.Errorf("Could not write registry images fo backup: %s", err)
			return err
		}
		for rImage := range rImages {
			if _, err := dfs.registry.PullImage(rImage); err != nil {
				glog.Errorf("Could not pull registry image %s: %s", rImage, err)
				return err
			}
			repotags = append(repotags, rImage)
		}
		select {
		case <-cancel:
			return ErrCancelled
		default:
		}
	}
	// Save images
	if err := dfs.docker.SaveImages(repotags, buffer); err != nil {
		glog.Errorf("Could not save images for backup: %s", err)
	} else if err := writeTar(tarfile, "images", buffer); err != nil {
		glog.Errorf("Could not write images for backup: %s", err)
		return err
	}
	return nil
}

func writeTar(tarfile *tar.Writer, name string, data *bytes.Buffer) error {
	header := &tar.Header{Name: name, Size: data.Len()}
	if err := tarfile.WriteHeader(header); err != nil {
		glog.Errorf("Could not create header %s: %s", header.Name, err)
		return err
	}
	if _, err := data.WriteTo(tarfile); err != nil {
		glog.Errorf("Could not write data to header %s: %s", header.Name, err)
		return err
	}
	if err := tarfile.Flush(); err != nil {
		glog.Errorf("Could not flush: %s", err)
		return err
	}
	return nil
}
