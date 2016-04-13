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
	"archive/tar"
	"bytes"
	"encoding/json"
	"io"
	"path"
	"time"

	"gopkg.in/pipe.v2"

	"github.com/control-center/serviced/commons/docker"
	"github.com/control-center/serviced/dfs/utils"
	"github.com/control-center/serviced/volume"
	"github.com/zenoss/glog"
)

const (
	BackupMetadataFile   = ".BACKUPINFO"
	SnapshotsMetadataDir = "SNAPSHOTS/"
	DockerImagesFile     = "IMAGES.dkr"
)

// Backup writes all application data into an export stream
func (dfs *DistributedFilesystem) Backup(data BackupInfo, w io.Writer) error {

	// A set of pipes that output tar streams
	var tarpipes []pipe.Pipe

	// write the backup metadata. This is too tiny to parallelize
	metadataBuffer := &bytes.Buffer{}
	if err := dfs.writeBackupMetadata(data, metadataBuffer); err != nil {
		glog.Errorf("Unable to write backup metadata: %s", err)
		return err
	}
	tarpipes = append(tarpipes, pipe.Read(metadataBuffer))

	var images []string
	// download the base images
	for _, image := range data.BaseImages {
		if _, err := dfs.docker.FindImage(image); docker.IsImageNotFound(err) {
			if err := dfs.docker.PullImage(image); docker.IsImageNotFound(err) {
				glog.Warningf("Could not pull base image %s, skipping", image)
				continue
			} else if err != nil {
				glog.Errorf("Could not pull image %s: %s", image, err)
				return err
			}
		} else if err != nil {
			glog.Errorf("Could not find image %s: %s", image, err)
			return err
		}
		glog.V(2).Infof("Prepared Docker image %s for backup", image)
		images = append(images, image)
	}
	// export the snapshots
	for _, snapshot := range data.Snapshots {
		vol, info, err := dfs.getSnapshotVolumeAndInfo(snapshot)
		if err != nil {
			glog.Errorf("Could not get snapshot info: %s", err)
			return err
		}
		// load the images from this snapshot
		glog.V(2).Infof("Preparing images for tenant %s", info.TenantID)
		r, err := vol.ReadMetadata(info.Label, ImagesMetadataFile)
		if err != nil {
			glog.Errorf("Could not receive images metadata for tenant %s: %s", info.TenantID, err)
			return err
		}
		var imgs []string
		if err := importJSON(r, &imgs); err != nil {
			glog.Errorf("Could not interpret images metadata for tenant %s: %s", info.TenantID, err)
			return err
		}
		timer := time.NewTimer(0)
		for _, img := range imgs {
			timer.Reset(dfs.timeout)
			if err := dfs.reg.PullImage(timer.C, img); err != nil {
				glog.Errorf("Could not pull image %s from registry: %s", img, err)
				return err
			}
			image, err := dfs.reg.ImagePath(img)
			if err != nil {
				glog.Errorf("Could not get the image path from registry %s: %s", img, err)
				return err
			}
			glog.V(2).Infof("Prepared Docker image %s for backup", image)
			images = append(images, image)
		}
		timer.Stop()
		prefix := path.Join(SnapshotsMetadataDir, info.TenantID, info.Label)
		tarpipes = append(tarpipes, pipe.Line(
			dfs.snapshotSavePipe(vol, info.Label),
			utils.PrefixPath(prefix),
		))
	}

	// Add the image save pipe
	tarpipes = append(tarpipes, pipe.Line(
		dfs.dockerSavePipe(images...),
		// Relocate the tar Docker spits out under a well-known directory
		// This directory is a tar file in older backups; restore should
		// switch on that to decide how to extract them
		utils.PrefixPath(DockerImagesFile),
	))

	backupPipeline := pipe.Line(
		utils.ConcatTarStreams(tarpipes...),
		pipe.Write(w),
	)
	glog.Infof("Beginning backup")
	err := pipe.Run(backupPipeline)
	if err != nil {
		glog.Errorf("Unable to complete backup: %s", err)
		return err
	}
	glog.Infof("Backup complete")
	return nil
}

// dockerSavePipe streams the tar archive output by Docker to pipe's stdout
func (dfs *DistributedFilesystem) dockerSavePipe(images ...string) pipe.Pipe {
	return pipe.TaskFunc(func(s *pipe.State) error {
		if err := dfs.docker.SaveImages(images, s.Stdout); err != nil {
			glog.Errorf("Unable to save Docker images: %s", err)
			return err
		}
		return nil
	})
}

// snapshotSavePipe returns a pipe that exports a given volume to the pipe's stdout
func (dfs *DistributedFilesystem) snapshotSavePipe(vol volume.Volume, label string) pipe.Pipe {
	return pipe.TaskFunc(func(s *pipe.State) error {
		if err := vol.Export(label, "", s.Stdout); err != nil {
			glog.Errorf("Unable to export snapshot: %s", err)
			return err
		}
		return nil
	})
}

// writeBackupMetadata writes out a tar stream containing a file containing the
// JSON-serialized backup metdata passed in
func (dfs *DistributedFilesystem) writeBackupMetadata(data BackupInfo, w io.Writer) error {
	var (
		jsonData []byte
		err      error
	)
	tarfile := tar.NewWriter(w)
	defer tarfile.Close()
	glog.Infof("Writing backup metadata")
	if jsonData, err = json.Marshal(data); err != nil {
		return err
	}
	header := &tar.Header{Name: BackupMetadataFile, Size: int64(len(jsonData))}
	if err := tarfile.WriteHeader(header); err != nil {
		glog.Errorf("Could not create metadata header for backup: %s", err)
		return err
	}
	if _, err := tarfile.Write(jsonData); err != nil {
		glog.Errorf("Could not write backup metadata: %s", err)
		return err
	}
	return nil
}
