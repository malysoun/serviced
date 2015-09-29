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
	"io"

	"github.com/control-center/serviced/commons"
)

type Docker interface {
	FindImage(image string) (*dockerclient.Image, error)
	SaveImages(images []string, writer io.Writer) error
	LoadImage(reader io.Reader) error
	PushImage(image string) error
	PullImage(image string) error
	TagImage(oldImage, newImage string) error
	RemoveImage(name string) error
	ImageHistory(image string) ([]dockerclient.ImageHistory, error)
	FindContainer(ctr string) (*dockerclient.Container, error)
	CommitContainer(ctr, image string) (*dockerclient.Image, error)
}

type docker struct {
	client docker.ClientInterface
}

func (d *docker) FindImage(image string) (*dockerclient.Image, error) {
	return d.client.InspectImage(image)
}

func (d *docker) SaveImages(images []string, writer io.Writer) error {
	opts := dockerclient.ExportImagesOptions{
		Names:        images,
		OutputStream: writer,
	}
	return d.client.ExportImages(opts)
}

func (d *docker) LoadImages(reader io.Reader) error {
	opts := dockerclient.LoadImageOptions{
		InputStream: reader,
	}
	return d.client.LoadImages(opts)
}

func (dfs *DistributeFilesystem) PushImage(image string) error {
	imageID, err := commons.ParseImageID(image)
	if err != nil {
		return err
	}
	opts := dockerclient.PushImageOptions{
		Name:     imageID.BaseName(),
		Tag:      imageID.Tag,
		Registry: imageID.Registry(),
	}
	creds := docker.FetchRegistryCreds(imageID.Registry())
	return d.client.PushImage(opts, creds)
}

func (d *docker) PullImage(image string) error {
	imageID, err := commons.ParseImageID(image)
	if err != nil {
		return err
	}
	opts := dockerclient.PullImageOptions{
		Repository: imageID.BaseName(),
		Registry:   imageID.Registry(),
		Tag:        imageID.Tag,
	}
	creds := docker.FetchRegistryCreds(imageID.Registry())
	return d.client.PullImage(opts, creds)
}

func (d *docker) TagImage(oldImage, newImage string) error {
	newImageID, err := commons.ParseImageID(newImage)
	if err != nil {
		return err
	}
	opts := dockerclient.TagImageOptions{
		Repo:  newImageID.BaseName(),
		Tag:   newImageID.Tag,
		Force: true,
	}
	return d.client.TagImage(oldImage, opts)
}

func (d *docker) RemoveImage(image string) error {
	return d.client.RemoveImage(image)
}

func (d *docker) ImageHistory(image string) ([]dockerclient.ImageHistory, error) {
	return d.client.ImageHistory(image)
}

func (d *docker) FindContainer(ctr string) (*dockerclient.Container, error) {
	return d.client.InspectContainer(ctr)
}

func (d *docker) CommitContainer(ctr string, image string) (*dockerclient.Image, error) {
	imageID, err := commons.ParseImageID(image)
	if err != nil {
		return nil, err
	}
	opts := dockerclient.CommitContainerOptions{
		Container:  ctr,
		Repository: imageID.Repository(),
		Tag:        imageID.Tag,
	}
	return d.client.CommitContainer(opts)
}
