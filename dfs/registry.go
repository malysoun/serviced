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
	"errors"
	"fmt"
	"time"

	"github.com/control-center/serviced/commons"
	"github.com/control-center/serviced/commons/docker"
	"github.com/control-center/serviced/datastore"
	"github.com/control-center/serviced/facade"
	"github.com/zenoss/glog"
)

// Registry is the current mapping of all docker registry tags to uuids
type Registry interface {
	GetImage(image string) (*registry.Image, error)
	PushImage(image, uuid string) (string, error)
	PullImage(image string) (string, error)
	DeleteImage(image string) error
	SearchLibrary(library string, tags ...string) (map[string]string, error)
}

// registry is the dfs implementation of registry where data is written into
// the facade.
type registry struct {
	host   string
	port   int
	facade *facade.Facade
	docker Docker
}

func (r *registry) GetImage(image string) (*registry.Image, error) {
	imageID, err := commons.ParseImageID(image)
	if err != nil {
		return nil, err
	}
	if imageID.IsLatest() {
		imageID.Tag = docker.DockerLatest
	}
	repotag := &commons.ImageID{
		User: imageID.User,
		Repo: imageID.Repo,
		Tag:  imageID.Tag,
	}.String()
	ctx := datastore.Get()
	rImage, err := r.facade.GetRegistryImage(ctx, repotag)
	if err != nil {
		return nil, err
	}
	return rImage, nil
}

func (r *registry) PushImage(image, uuid string) (string, error) {
	imageID, err := commons.ParseImageID(image)
	if err != nil {
		return "", err
	}
	if imageID.IsLatest() {
		imageID.Tag = docker.DockerLatest
	}
	repotag := &commons.ImageID{
		Host: r.host,
		Port: r.port,
		User: imageID.User,
		Repo: imageID.Repo,
		Tag:  imageID.Tag,
	}.String()
	if err := r.docker.TagImage(uuid, repotag); err != nil {
		return "", err
	}
	rImage := &registry.Image{
		Library: imageID.User,
		Repo:    imageID.Repo,
		Tag:     imageID.Tag,
		UUID:    imageID.UUID,
	}
	ctx := datastore.Get()
	if err := r.facade.SetRegistryImage(ctx, rImage); err != nil {
		return "", err
	}
	go func() {
		if err := r.docker.PushImage(repotag); err != nil {
			glog.Warningf("Could not push image %s: %s", repotag, err)
			return
		}
		rImage.PushedAt = time.Now().UTC()
		if err := r.facade.SetRegistryImage(ctx, rImage); err != nil {
			glog.Warningf("Could not update registry %s: %s", repotag, err)
			return
		}
	}()
	return repotag, nil
}

func (r *registry) PullImage(image string) (string, error) {
	imageID, err := commons.ParseImageID(img.Config.Image)
	if err != nil {
		return "", err
	}
	if imageID.IsLatest() {
		imageID.Tag = docker.DockerLatest
	}
	repotag := &commons.ImageID{
		User: imageID.User,
		Repo: imageID.Repo,
		Tag:  imageID.Tag,
	}.String()
	ctx := datastore.Get()
	rImage, err := r.facade.GetRegistryImage(ctx, repotag)
	if err != nil {
		return "", err
	}
	img, err := r.docker.FindImage(rImage.UUID)
	if err != nil {
		if docker.IsImageNotFound(err) {
			if rImage.PushedAt > 0 {
				if err := r.docker.PullImage(fmt.Sprintf("%s:%d/%s", r.host, r.port, repotag)); err != nil {
					return err
				}
				if img, err = r.docker.FindImage(rImage.UUID); err != nil {
					return err
				}
				return rImage.UUID, nil
			} else {
				return "", errors.New("dfs: image not in registry")
			}
		} else {
			return "", err
		}
	}
	if err := docker.TagImage(rImage.UUID, fmt.Sprintf("%s:%d/%s", r.host, r.port, repotag)); err != nil {
		return "", err
	}
	return rImage.UUID, nil
}

func (r *registry) DeleteImage(image string) error {
	imageID, err := commons.ParseImageID(img.Config.Image)
	if err != nil {
		return "", err
	}
	if imageID.IsLatest() {
		imageID.Tag = docker.DockerLatest
	}
	repotag := &commons.ImageID{
		User: imageID.User,
		Repo: imageID.Repo,
		Tag:  imageID.Tag,
	}.String()
	ctx := datastore.Get()
	rImage, err := r.facade.DeleteRegistryImage(ctx, repotag)
	if err != nil {
		return err
	}
	r.docker.RemoveImage(fmt.Sprintf("%s:%d/%s", r.host, r.port, repotag))
	return nil
}

func (r *registry) SearchLibrary(library, tags ...string) (map[string]string, error) {
	images, err := f.facade.SearchRegistryLibrary(ctx, library, tags)
	if err != nil {
		return nil, err
	}
	repotags := make(map[string]string)
	for _, image := range repotags {
		repotags[fmt.Sprintf("%s:%d/%s", r.host, r.port, image)] = image.UUID
	}
	return repotags, nil
}
