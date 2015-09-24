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

package dfs

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/control-center/serviced/commons"
	"github.com/control-center/serviced/commons/docker"
	"github.com/control-center/serviced/domain/service"
	"github.com/control-center/serviced/volume"
	"github.com/zenoss/glog"
)

type DFS interface {
	Create(tenantID string) error
	Destroy(tenantID string) error
	Snapshot(tenantID, message string) (string, error)
	Rollback(snapshotID string) error
	Commit(dockerID, message string) error
	GetSnapshotInfo(snapshotID string) (*SnapshotInfo, err)
	ListSnapshots(tenantID string) ([]string, error)
	DeleteSnapshot(snapshotID string)
	Backup(writer io.Writer) error
	Restore(reader io.Reader) error
	Lock() error
	Unlock() error
}

// DistributedFilesystem manages storage and docker distribution of services
type DistributedFilesystem struct {
	data    Database
	docker  Docker
	disk    volume.Driver
	net     volume.Network
	timeout time.Duration
	rHost   string
	rPort   int
}

// Create sets up the volume and the registry for its images
func (dfs *DistributedFilesystem) Create(tenantID string) error {
	// get the services
	svcs, err := dfs.data.GetServices(tenantID)
	if err != nil {
		glog.Errorf("Could not get services of tenant %s: %s", tenantID, err)
		return err
	}
	// build the volume
	vol, err := dfs.disk.Create(tenantID)
	if err != nil {
		glog.Errorf("Could not initialize volume for tenant %s: %s", tenantID, err)
		return err
	}
	// add the volume to the share
	if err := dfs.net.AddShare(tenantID, vol.Path()); err != nil {
		glog.Errorf("Could not add share for volume %s: %s", tenantID, err)
		return err
	}
	// set up the registry
	images := make(map[string]string)
	for _, svc := range svcs {
		registryImageID, ok := images[svc.ImageID]
		if !ok {
			registryImageID, err = dfs.createRepo(tenantID, svc.ImageID)
			if err != nil {
				glog.Errorf("Could not initialize image %s of tenant %s to the registry: %s", svc.ImageID, tenantID, err)
				return err
			}
		}
		svc.ImageID = registryImageID
		if err := dfs.data.UpdateService(svc); err != nil {
			glog.Errorf("Could not update image %s for service %s (%s)", svc.ImageID, svc.Name, svc.ID, err)
			return err
		}
	}
	// enable networking for tenant
	if err := dfs.net.EnableShare(tenantID); err != nil {
		glog.Errorf("Could not enable sharing for %s: %s", tenantID, err)
		return err
	}
	return nil
}

func (dfs *DistributedFilesyste) parseRepoTag(repotag string) (*commons.ImageID, error) {
	imageID, err := commons.ParseImageID(repotag)
	if err != nil {
		glog.Errorf("Could not parse image %s: %s", repotag, err)
		return nil, err
	}
	imageID.Host, imageID.Port = dfs.rHost, dfs.rPort
	if imageID.IsLatest() {
		imageID.Tag = docker.DockerLatest
	}
	return imageID, nil
}

func (dfs *DistributedFilesystem) PushImage(repotag, uuid string) error {
	imageID, err := dfs.parseRepoTag(repotag)
	if err != nil {
		return err
	}
	if err := dfs.docker.TagImage(uuid, imageID.String()); err != nil {
		glog.Errorf("Could not set tag %s on uuid %s: %s", repotag, uuid, err)
		return err
	}
	if err := dfs.data.SetRegistryImage(imageID, uuid); err != nil {
		glog.Errorf("Could not update registry with tag %s: %s", repotag, err)
		return err
	}
	go func() {
		if err := dfs.docker.PushImage(imageID.String()); err != nil {
			glog.Errorf("Could not push tag %s into registry: %s", repotag, err)
			return
		}
		if err := dfs.data.SetRegistryPush(imageID); err != nil {
			glog.Errorf("Could not update tag %s in registry: %s", repotag, err)
			return
		}
	}()
	return nil
}

func (dfs *DistributedFilesystem) PullImage(repotag string) (*dockerclient.Image, error) {
	var imageID *commons.ImageID
	var uuid string

	image, err := dfs.docker.FindImage(repotag)
	if err != nil {
		if docker.IsImageNotFound(err) {
			if imageID, err = dfs.parseRepoTag(repotag); err != nil {
				return nil, err
			}
		} else {
			glog.Errorf("Could not look up image %s: %s", repotag, err)
			return err
		}
	} else {
		imageID, uuid = image.ImageID, image.UUID
	}
	registryImage, err := dfs.data.GetRegistryImage(imageID)
	if err != nil {
		glog.Errorf("Could not find image %s in registry: %s", imageID, err)
		return nil, err
	}
	if registryImage.UUID != uuid {
		if registryImage.LastPush.Unix() > 0 {
			if image, err = dfs.docker.PullImage(imageID.String()); err != nil {
				glog.Errorf("Could not pull image %s: %s", repotag, err)
				return nil, err
			} else {
				return nil, errors.New("image not pushed")
			}
		}
	}
	return imageID, nil
}

// createRepo builds the repo for the tenant
func (dfs *DistributedFilesystem) createRepo(tenantID, imageID string) (string, error) {
	// Download the image if it isn't already there
	img, err := dfs.docker.FindImage(imageID)
	if err != nil {
		if docker.IsImageNotFound(err) {
			if img, err = dfs.docker.PullImage(image); err != nil {
				glog.Errorf("Could not pull image %s: %s", imageID, err)
				return "", err
			}
		} else {
			glog.Errorf("Could not look up image %s: %s", imageID, err)
			return "", err
		}
	}
	registryImage := &commons.ImageID{
		User: tenantID,
		Repo: img.ImageID.Repo,
		Tag:  docker.DockerLatest,
	}
	if err := dfs.PushImage(registryImage.String(), img.UUID); err != nil {
		return "", err
	}
	return registryImage.String(), nil
}

// Destroy destroys a volume and related images in the registry
func (dfs *DistributedFilesystem) Destroy(tenantID string) error {
	// Prevent service scheduler from creating new instances
	lock := dfs.data.GetTenantLock(tenantID)
	if err != nil {
		glog.Errorf("Could not get lock for tenant %s: %s", tenantID, err)
		return "", err
	}
	if err := lock.Lock(); err != nil {
		glog.Errorf("Could not lock services for tenant %s: %s", tenantID, err)
		return "", err
	}
	defer lock.Unlock()
	// Get all the services
	svcs, err := dfs.data.GetServices(tenantID)
	if err != nil {
		glog.Errorf("Could not get services for tenant %s: %s", tenantID, err)
		return "", err
	}
	// Wait for services to stop, unless they are set to run, otherwise bail out
	serviceIDs := make([]string, len(svcs))
	for i, svc := range svcs {
		if svc.DesiredState != int(service.SVCStop) {
			return errors.New("dfs: found running services")
		}
		serviceIDs[i] = svc.ID
	}
	if err := dfs.data.WaitServices(service.SVCStop, dfs.timeout, serviceIDs); err != nil {
		glog.Errorf("Could not %s services for tenant %s: %s", service.SVCStop, tenantID, err)
		return "", err
	}
	// Stop sharing the tenant
	if err := dfs.net.DisableShare(tenantID); err != nil {
		glog.Errorf("Could not disable sharing for %s: %s", tenantID, err)
		return err
	}
	// Remove the share
	if err := dfs.net.RemoveShare(tenantID); err != nil {
		glog.Errorf("Could not remove share for %s: %s", tenantID, err)
		return err
	}
	// Remove the disk
	if err := dfs.disk.Remove(tenantID); err != nil {
		glog.Errorf("Could not remove volume %s: %s", tenantID, err)
		return err
	}
	// Destroy the image repo from the registry
	if err := dfs.destroyLibrary(tenantID); err != nil {
		glog.Errorf("Could not destroy repo %s: %s", tenantID, err)
		return err
	}
	return nil
}

// destroyRepo removes all images of a tenant from the registry
func (dfs *DistributedFilesystem) destroyLibrary(tenantID string) error {
	if err := dfs.data.DeleteRegistryLibrary(tenantID); err != nil {
		glog.Errorf("Could not remove registry library for %s: %s", tenantID, err)
		return err
	}
	return nil
}

// Snapshot takes a snapshot of a tenants services, volumes, and images
func (dfs *DistributedFilesystem) Snapshot(tenantID, message string) (string, error) {
	// Prevent service scheduler from creating new instances
	lock := dfs.data.GetTenantLock(tenantID)
	if err != nil {
		glog.Errorf("Could not get lock for tenant %s: %s", tenantID, err)
		return "", err
	}
	if err := lock.Lock(); err != nil {
		glog.Errorf("Could not lock services for tenant %s: %s", tenantID, err)
		return "", err
	}
	defer lock.Unlock()
	// Get all the services
	svcs, err := dfs.data.GetServices(tenantID)
	if err != nil {
		glog.Errorf("Could not get services for tenant %s: %s", tenantID, err)
		return "", err
	}
	// Get the tenant volume
	vol, err := dfs.disk.Get(tenantID)
	if err != nil {
		glog.Errorf("Could not get volume for tenant %s: %s", tenantID, err)
		return "", err
	}
	// Pause running services
	serviceIDs := make([]string, len(svcs))
	for i, svc := range svcs {
		if svc.DesiredState == int(service.SVCRun) {
			defer dfs.data.ScheduleService(svc.ID, false, service.SVCRun)
			if _, err := dfs.data.ScheduleService(svc.ID, false, service.SVCPause); err != nil {
				glog.Errorf("Could not %s service %s (%s): %s", service.SVCPause, svc.Name, svc.ID, err)
				return "", err
			}
		}
		serviceIDs[i] = svc.ID
	}
	if err := dfs.data.WaitServices(service.SVCPause, dfs.timeout, serviceIDs); err != nil {
		glog.Errorf("Could not %s services for tenant %s: %s", service.SVCPause, tenantID, err)
		return "", err
	}
	// Snapshot the image repository
	if err := dfs.tagRepo(tenantID, docker.DockerLatest, label); err != nil {
		glog.Errorf("Could not snapshot repo %s: %s", tenantID, err)
		return "", err
	}
	// Take a snapshot
	label := time.Now().UTC().TimeFormat(timeFormat)
	if err := vol.Snapshot(label, message); err != nil {
		glog.Errorf("Could not snapshot volume for tenant %s: %s", tenantID, err)
		return "", err
	}
	// Write service metadata
	if exportFile, err := vol.WriteMetadata(label, "services.json"); err != nil {
		glog.Errorf("Could not write service metadata for snapshot %s of tenant %s: %s", label, tenantID, err)
		return "", err
	} else if err := exportMetadata(exportFile, svcs); err != nil {
		glog.Errorf("Could not export services to label %s of tenant %s: %s", label, tenantID, err)
		return "", err
	}
	return tenantID + "_" + label, nil
}

// tagRepo updates the tag for an existing repository
func (dfs *DistributedFilesystem) tagRepo(tenantID, oldLabel, newLabel string) error {
	registryImages, err := dfs.data.SearchRegistryLibraryByTag(tenantID, oldLabel)
	if err != nil {
		glog.Errorf("Could not get repos from library %s with tag %s: %s", tenantID, oldLabel, err)
		return err
	}
	for _, registryImage := range registryImages {
		if _, err := dfs.PullImage(registryImage.String()); err != nil {
			glog.Errorf("Could not pull image %s: %s", registryImage, err)
			return err
		}
		registryImage.Tag = newLabel
		if err := dfs.PushImage(registryImage.String(), registryImage.UUID); err != nil {
			glog.Errorf("Could not push image %s (%s): %s", registryImage, registryImage.UUID, err)
			return err
		}
	}
	return nil
}

func (dfs *DistributedFilesystem) Commit(dockerID, message string) (string, error) {
	// Get the container
	ctr, err := dfs.data.FindContainer(dockerID)
	if err != nil {
		glog.Errorf("Could not get the container %s: %s", dockerID, err)
		return err
	}
	if ctr.IsRunning() {
		return errors.New("dfs: cannot commit a running container")
	}
	registryImage, err := dfs.data.GetRegistryImage(ctr.Image)
	if err != nil {
		glog.Errorf("Could not find image %s in registry: %s", ctr.Image, err)
		return err
	}
	if registryImage.Tag != docker.DockerLatest || registryImage.UUID != ctr.Config.Image {
		return errors.New("dfs: cannot commit a stale container")
	}
	image, err := dfs.docker.CommitContainer(ctr.ID, registryImage.String())
	if err != nil {
		glog.Errorf("Could not commit container %s: %s", dockerID, err)
		return err
	}
	if err := dfs.PushImage(registryImage.String(), image.ID); err != nil {
		glog.Errorf("Could not push image %s (%s): %s", registryImage, image.ID, err)
		return err
	}
	return dfs.Snapshot(registryImage.Library, message)
}

// Rollback reverts a tenant's services, volume, and repos to the state of the snapshot
func (dfs *DistributedFilesystem) Rollback(snapshotID string, forceRestart bool) error {
	// Look up the snapshot
	snapshot, err := dfs.disk.Get(snapshotID)
	if err != nil {
		glog.Errorf("Could not find snapshot %s: %s", snapshotID, err)
		return err
	}
	tenantID, label := snapshot.TenantID(), snapshot.Name()
	// Prevent service scheduler from creating new instances
	lock := dfs.data.GetTenantLock(tenantID)
	if err != nil {
		glog.Errorf("Could not get lock for tenant %s: %s", tenantID, err)
		return "", err
	}
	if err := lock.Lock(); err != nil {
		glog.Errorf("Could not lock services for tenant %s: %s", tenantID, err)
		return "", err
	}
	defer lock.Unlock()
	// Get all the services and make sure that they are stopped
	svcs, err := dfs.data.GetServices(snapshot.Tenant())
	if err != nil {
		glog.Errorf("Could not get services for tenant %s: %s", tenantID, err)
		return err
	}
	serviceIDs := make([]string, len(svcs))
	for i, svc := range svcs {
		if svc.DesiredState != int(service.SVCStop) {
			if forceRestart {
				defer dfs.data.ScheduleService(svc.ID, false, service.DesiredState(svc.DesiredState))
				if _, err := dfs.data.ScheduleService(svc.ID, false, service.SVCStop); err != nil {
					glog.Errorf("Could not %s service %s (%s)", service.SVCStop, svc.Name, svc.ID, err)
					return err
				}
			} else {
				return errors.New("dfs: found running services")
			}
		}
		serviceIDs[i] = svc.ID
	}
	if err := dfs.data.WaitServices(service.SVCStop, dfs.timeout, serviceIDs); err != nil {
		glog.Errorf("Could not %s service for tenant %s: %s", service.SVCStop, tenantID, err)
		return err
	}
	// Rollback the services
	vol, err := dfs.disk.Get(tenantID)
	if err != nil {
		glog.Errorf("Could not get volume for tenant %s: %s", tenantID, err)
		return err
	}
	if importFile, err := vol.ReadMetadata(label, "services.json"); err != nil {
		glog.Errorf("Could not read service metadata of snapshot %s: %s", snapshotID, err)
		return err
	} else if err := importMetadata(importFile, &svcs); err != nil {
		glog.Errorf("Could not import services from snapshot %s: %s", snapshotID, err)
		return err
	}
	if err := dfs.data.RestoreServices(tenantID, svcs); err != nil {
		glog.Errorf("Could not restore services from snapshot %s: %s", snapshotID, err)
		return err
	}
	// Rollback the images
	if err := dfs.tagRepo(tenantID, label, docker.DockerLatest); err != nil {
		glog.Errorf("Could not rollback repo %s: %s", tenantID, err)
		return err
	}
	// Stop sharing the tenant
	if err := dfs.net.DisableShare(tenantID); err != nil {
		glog.Errorf("Could not disable sharing for %s: %s", tenantID, err)
		return err
	}
	defer dfs.net.EnableShare(tenantID)
	// Rollback the volume
	if err := vol.Rollback(label); err != nil {
		glog.Errorf("Could not rollback snapshot %s for tenant %s: %s", tenantID, err)
		return err
	}
	return nil
}

func (dfs *DistributedFilesystem) Backup(writer io.Writer) error {
	tarfile := tar.NewWriter(writer)
	// Store the fstype
	if err := writetar(tarfile, ".METADATA/fstype", []byte(dfs.disk.DriverType())); err != nil {
		glog.Errorf("Could not write: %s", err)
		return err
	}
	// Get the resource pools
	if pools, err := dfs.data.GetResourcePools(); err != nil {
		glog.Errorf("Could not get resource pools for backup: %s", err)
		return err
	} else if data, err := json.Marshal(pools); err != nil {
		glog.Errorf("Could not marshal resource pools for backup: %s", err)
		return err
	} else if err := writetar(tarfile, ".METADATA/pools", data); err != nil {
		glog.Errorf("Could not write resource pools for backup: %s", err)
		return err
	}
	// Get the hosts
	if hosts, err := dfs.data.GetHosts(); err != nil {
		glog.Errorf("Could not get hosts for backup: %s", err)
		return err
	} else if data, err := json.Marshal(hosts); err != nil {
		glog.Errorf("Could not marshal hosts for backup: %s", err)
		return err
	} else if err := writetar(tarfile, ".METADATA/hosts", data); err != nil {
		glog.Errorf("Could not write hosts for backup: %s", err)
		return err
	}
	// Get the service templates
	if templates, err := dfs.data.GetServiceTemplates(); err != nil {
		glog.Errorf("Could not get service templates for backup: %s", err)
		return err
	} else if data, err := json.Marshal(templates); err != nil {
		glog.Errorf("Could not marshal service templates for backup: %s", err)
		return err
	} else if err := writetar(tarfile, ".METADATA/templates", data); err != nil {
		glog.Errorf("Could not write service templates for backup: %s", err)
		return err
	}
	repotags, err := dfs.data.GetServiceTemplateImages()
	if err != nil {
		glog.Errorf("Could not get images for service templates: %s", err)
		return err
	}
	// Get the tenant services
	tenants, err := dfs.data.GetTenantServices()
	if err != nil {
		glog.Errorf("Could not get tenant services for backup: %s", err)
		return err
	} else if data, err := json.Marshal(tenants); err != nil {
		glog.Errorf("Could not marshal tenant services for backup: %s", err)
		return err
	} else if err := writetar(tarfile, "./.METADATA/services", data); err != nil {
		glog.Errorf("Could not write tenant services for backup: %s", err)
		return err
	}
	// Snapshot each tenant
	for _, tenant := range tenants {
		vol, err := dfs.disk.Get(tenant.ID)
		if err != nil {
			glog.Errorf("Could not get volume for tenant %s: %s", tenant.ID, err)
			return err
		}
		snapshotID, err := dfs.Snapshot(tenant.ID, "")
		if err != nil {
			glog.Errorf("Could not snapshot %s: %s", tenant.ID, err)
			return err
		}
		defer dfs.DeleteSnapshot(snapshotID)
		snapshot, err := vol.SnapshotInfo(snapshotID)
		if err != nil {
			glog.Errorf("Could not get snapshot info for tenant %s: %s", tenant.ID, err)
			return err
		}
		buffer := bytes.NewBuffer([]byte{})
		// export the snapshot
		if err := vol.Export(snapshot.Label, "", buffer); err != nil {
			glog.Errorf("Could not export snapshot %s: %s", snapshotID, err)
			return err
		} else if err := writetar(tarfile, fmt.Sprintf("./apps/%s", tenant.ID), buffer.Bytes()); err != nil {
			glog.Errorf("Could not write snapshot for backup: %s", err)
			return err
		}
		// export the snapshot images
		registryImages, err := dfs.data.SearchRegistryLibraryByTag(tenant.ID, snapshot.Label)
		if err != nil {
			glog.Errorf("Could not get registry images for %s at %s: %s", tenant.ID, snapshot.Label, err)
			return err
		}
		for _, registryImage := range registryImages {
			imageID, err := dfs.PullImage(registryImage.String())
			if err != nil {
				glog.Errorf("Could not pull registry image %s: %s", registryImage, err)
				return err
			}
			repotags = append(repotags, imageID)
		}
	}
	// Save images
	buffer := bytes.NewBuffer([]byte{})
	if err := dfs.docker.SaveImages(repotags, buffer); err != nil {
		glog.Errorf("Could not save images for backup: %s", err)
	} else if err := writetar(tarfile, "./images", buffer.Bytes()); err != nil {
		glog.Errorf("Could not write images for backup: %s", err)
		return err
	}
	return nil
}

func writetar(tarfile *tar.Writer, name string, data []byte) error {
	header := &tar.Header{Name: name, Size: len(data)}
	if err := tarfile.WriteHeader(header); err != nil {
		glog.Errorf("Could not create header %s: %s", header.Name, err)
		return err
	}
	if _, err := tarfile.Write(data); err != nil {
		glog.Errorf("Could not write data to header %s: %s", header.Name, err)
		return err
	}
	return nil
}
