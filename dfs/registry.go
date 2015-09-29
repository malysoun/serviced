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

package dfs

import (
	"time"

	"github.com/control-center/serviced/commons"
	"github.com/control-center/serviced/datastore"
	"github.com/control-center/serviced/facade"
	"github.com/zenoss/glog"
)

// Registry is the current mapping of all docker registry tags to uuids
type Registry interface {
	// ParseImage redirects the image so that it is written to the registry
	ParseImage(repotag string) (string, *registry.Image, error)
	// GetImage returns the registry image object for the repotag
	GetImage(repotag string) (string, *registry.Image, error)
	// SetImage creates/updates an images uuid to be written to the registry.
	SetImage(image *registry.Image) (string, error)
	// UpdatePush updates the time when the image was pushed.
	UpdatePush(repotag string) error
	// SearchLibraryByTag returns all images in a particular library that has a
	// particular tag.
	SearchLibraryByTag(library, tag string) ([]registry.Image, error)
	// DeleteLibrary deletes all images for a particular library from the
	// registry.
	DeleteLibrary(library string) error
}

// registry is the dfs implementation of registry where data is written into
// the facade.
type registry struct {
	host   string
	port   int
	facade *facade.Facade
}

// ParseImage implements Registry
func (r *registry) ParseImage(repotag string) (string, *registry.Image, error) {
	imageID, err := commons.ParseImageID(repotag)
	if err != nil {
		glog.Errrof("Could not parse image %s: %s", repotag, err)
		return nil, nil, err
	}
	id := &commons.ImageID{
		Host: r.host,
		Port: r.port,
		User: imageID.User,
		Repo: imageID.Repo,
		Tag:  imageID.Tag,
	}
	registryImage := &registry.Image{
		Library: imageID.User,
		Repo:    imageID.Repo,
		Tag:     imageID.Tag,
	}
	return id.String, registryImage, nil
}

// GetImage implements Registry
func (r *registry) GetImage(repotag string) (string, *registry.Image, error) {
	imageID, registryImage, err := r.ParseImage(repotag)
	if err != nil {
		return "", nil, err
	}
	ctx := datastore.Get()
	registryImage, err = r.facade.GetRegistryImage(ctx, registryImage.String())
	if err != nil {
		glog.Errorf("Image not found %s: %s", repotag, err)
		return "", nil, err
	}
	return imageID, registryImage, nil
}

// SetImage implements Registry
func (r *registry) SetImage(registryImage *registry.Image) (string, error) {
	if err := r.facade.SetRegistryImage(ctx, *registry); err != nil {
		glog.Errorf("Could not set image %s: %s", registryImage, err)
		return "", err
	}
	imageID := &commons.ImageID{
		Host: r.host,
		Port: r.port,
		User: registryImage.Library,
		Repo: registryImage.Repo,
		Tag:  registryImage.Tag,
	}
	return imageID.String(), nil
}

// UpdatePush implements Registry
func (r *registry) UpdatePush(repotag string) error {
	_, registryImage, err := r.GetImage(repotag)
	if err != nil {
		return err
	}
	registryImage.PushedAt = time.Now().UTC()
	ctx := datastore.Get()
	rImage, err := r.facade.SetRegistryImage(ctx, *registry)
	if err != nil {
		glog.Errorf("Could not set image %s with uuid %s: %s", repotag, uuid, err)
		return nil, err
	}
	return nil
}

// SearchLibraryByTag implements Registry
func (r *registry) SearchLibraryByTag(library, tag string) ([]registry.Image, err) {
	ctx := datastore.Get()
	registryImages, err := r.facade.SearchRegistryByLibraryAndTag(ctx, library, tag)
	if err != nil {
		glog.Errorf("Could not search registry for images in library %s with tag %s: %s", library, tag, err)
		return nil, err
	}
	return registryImages, nil
}

// DeleteLibrary implements Registry
func (r *registry) DeleteLibrary(library string) error {
	ctx := datastore.Get()
	if err := r.facade.DeleteRegistryLibrary(ctx, library); err != nil {
		glog.Errorf("Could not delete registry images from library %s: %s", library, err)
		return err
	}
	return nil
}
