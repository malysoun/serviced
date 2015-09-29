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
	"encoding/json"
	"errors"
	"io"
	"path"
	"strings"

	"github.com/control-center/serviced/domain/host"
	"github.com/control-center/serviced/domain/pool"
	"github.com/control-center/serviced/domain/servicetemplate"
	"github.com/control-center/serviced/volume"
	"github.com/zenoss/glog"
)

var (
	ErrObjectInit    = errors.New("object initialized")
	ErrObjectNotInit = errors.New("object not initialized")
)

type restore struct {
	Templates chan []servicetemplate.ServiceTemplate
	Pools     chan []pool.ResourcePool
	Hosts     chan []host.Host
	Tenants   map[string]string
	Registry  map[string]chan []RegistryImage
}

func (r *restore) init() *restore {
	r = &restore{
		Templates: make(chan []servicetemplate.ServiceTemplate, 1),
		Pools:     make(chan []pool.ResourcePool, 1),
		Hosts:     make(chan []host.Host, 1),
		Tenants:   make(map[string]string),
		Registry:  make(map[string]chan []registry.RegistryImage),
	}
	return r
}

// Restore rolls back the entire control center database from a backup.
func (dfs *DistributedFilesystem) Restore(reader io.Reader) error {
	r := new(restore).init()

	// load the stream as a tarfile
	tarfile := tar.NewReader(reader)
	for {
		header, err := tarfile.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			glog.Errorf("Could not read header from backup: %s", err)
			return err
		}
		switch header.Name {
		case "DATA/templates":
			var templates []servicetemplate.ServiceTemplate
			if err := json.NewDecoder(tarfile).Decode(&templates); err != nil {
				glog.Errorf("Could not decode service templates from backup: %s", err)
				return err
			}
			select {
			case r.Templates <- templates:
				glog.Infof("Loaded %d templates from backup", len(templates))
			default:
				glog.Errorf("Could not load templates from backup: %s", ErrObjectInit)
				return ErrObjectInit
			}
		case "DATA/pools":
			var pools []pool.ResourcePool
			if err := json.NewDecoder(tarfile).Decode(&pools); err != nil {
				glog.Errorf("Could not decode resource pools from backup: %s", err)
				return err
			}
			select {
			case r.Pools <- pools:
				glog.Infof("Loaded %d resource pools from backup", len(pools))
			default:
				glog.Errorf("Could not load resource pools from backup: %s", ErrObjectInit)
				return ErrObjectInit
			}
		case "DATA/hosts":
			var hosts []host.Host
			if err := json.NewDecoder(tarfile).Decode(&hosts); err != nil {
				glog.Errorf("Could not decode hosts from backup: %s", err)
				return err
			}
			select {
			case r.Hosts <- hosts:
				glog.Infof("Loaded %d hosts from backup", len(hosts))
			default:
				glog.Errorf("Could not load resource pools from backup: %s", ErrObjectInit)
				return ErrObjectInit
			}
		case "images":
			if err := dfs.docker.LoadImage(tarfile); err != nil {
				glog.Errorf("Could not load docker images from backup: %s", err)
				return err
			}
		default:
			if strings.HasPrefix(header.Name, "APPS/") {
				appName := strings.TrimPrefix(header.Name, "APPS/")
				tenantID, base := path.Dir(app), path.Base(app)
				regChan, ok := r.Registry[tenantID]
				if !ok {
					r.Registry[tenantID] = make(chan []registry.RegistryImage, 1)
					regChan = r.Registry[tenantID]
				}
				switch base {
				case "volume":
					var vol volume.Volume
					if vol, err = dfs.disk.Create(tenantID); err != nil {
						if err == volume.ErrVolumeExists {
							if vol, err = dfs.disk.Get(tenantID); err != nil {
								glog.Errorf("Could not look up tenant volume %s: %s", tenantID, err)
								return err
							}
							glog.Errorf("Could not create tenant volume %s: %s", tenantID, err)
							return err
						}
					}
					label, err := vol.Import(tarfile)
					if err != nil {
						glog.Errorf("Could not read snapshot for %s from backup: %s", tenantID, err)
						return err
					}
					t.Tenants[tenantID] = label
				case "registry":
					var registry []registry.RegistryImage
					if err := json.NewDecoder(tarfile).Decode(&registry); err != nil {
						glog.Errorf("Could not decode registry data for tenant %s from backup: %s", tenantID, err)
						return err
					}
					select {
					case regChan <- registry:
						glog.Infof("Loaded %d registry images for tenant %s", len(registry), tenantID)
					default:
						glog.Errorf("Could not load resource pools from backup: %s", ErrObjectInit)
						return ErrObjectInit
					}
				}
			}
		}
	}
	// restore backup data

	// restore service templates
	select {
	case templates := <-r.Templates:
		if err := dfs.data.RestoreServiceTemplates(templates); err != nil {
			glog.Errorf("Could not restore service templates from backup: %s", err)
			return err
		}
	default:
		glog.Errorf("Service templates not found: %s", ErrObjectNotInit)
		return ErrObjectNotInit
	}

	// restore resource pools
	select {
	case pools := <-r.Pools:
		if err := dfs.data.RestoreResourcePools(pools); err != nil {
			glog.Errorf("Could not restore resource pools from backup: %s", err)
			return err
		}
	default:
		glog.Errorf("Resource pools not found: %s", ErrObjectNotInit)
		return ErrObjectNotInit
	}

	// restore hosts
	select {
	case hosts := <-r.Hosts:
		if err := dfs.data.RestoreHosts(hosts); err != nil {
			glog.Errorf("Could not restore hosts from backup: %s", err)
			return err
		}
	default:
		glog.Errorf("Hosts not found: %s", ErrObjectNotInit)
		return ErrObjectNotInit
	}

	// restore disk and images
	for tenantID, label := range r.Tenants {
		select {
		case registryImages := <-r.Registry[tenantID]:
			for _, registryImage := range registryImages {
				if err := dfs.PushImage(registryImage.String(), registryImage.UUID); err != nil {
					glog.Errorf("Could not restore images for tenant %s from backup: %s", tenantID, err)
					return err
				}
			}
		default:
			glog.Errorf("Registry images not found: %s", ErrObjectNotInit)
			return ErrObjectNotInit
		}
		if err := dfs.disk.Rollback(label, false); err != nil {
			glog.Errorf("Could not restore application data for tenant %s: %s", tenantID, err)
			return err
		}
	}

	return nil
}
