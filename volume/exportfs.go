// Copyright 2015 The Serviced Authors.
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

package volume

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/zenoss/glog"
)

// ExportDirectory recursively writes its contents into a tar Writer.
func ExportDirectory(tarfile *tar.Writer, path, name string) error {
	dir, err := os.Open(path)
	if err != nil {
		glog.Errorf("Could not open %s: %s", path, err)
		return err
	}
	defer dir.Close()
	fstat, err := dir.Stat()
	if err != nil {
		glog.Errorf("Could not stat %s: %s", path, err)
		return err
	}
	header, err := getHeader(name, "", fstat)
	if err != nil {
		return err
	}
	if err := tarfile.WriteHeader(header); err != nil {
		glog.Errorf("Could not write header for directory %s: %s", path, err)
		return err
	}
	files, err := dir.Readdir(0)
	if err != nil {
		glog.Errorf("Could not list directory for %s: %s", path, err)
		return err
	}
	for _, finfo := range files {
		fullpath, relpath := filepath.Join(path, finfo.Name()), filepath.Join(name, finfo.Name())
		if finfo.IsDir() {
			if err := ExportDirectory(tarfile, fullpath, relpath); err != nil {
				return err
			}
		} else {
			if err := ExportFile(tarfile, fullpath, relpath); err != nil {
				return err
			}
		}
	}
	return nil
}

// ExportFile writes a file into a tar Writer.
func ExportFile(tarfile *tar.Writer, path, name string) error {
	finfo, _ := os.Stat(path)
	if isSocket := finfo.Mode() & os.ModeSocket; isSocket == os.ModeSocket {
		glog.Warningf("Cannot export Unix domain socket %s", path)
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		glog.Errorf("Could not open %s: %s", path, err)
		return err
	}
	defer file.Close()
	fstat, err := file.Stat()
	if err != nil {
		glog.Errorf("Could not stat %s: %s", path, err)
		return err
	}

	link, err := filepath.EvalSymlinks(path)
	if err != nil {
		glog.Errorf("Could not check link for %s: %s", path, err)
		return err
	}
	header, err := getHeader(name, link, fstat)
	if err != nil {
		glog.Errorf("Could not create file header %s: %s", path, err)
		return err
	}
	if err := tarfile.WriteHeader(header); err != nil {
		glog.Errorf("Could not write file header %s: %s", path, err)
		return err
	}
	if link == path {
		if _, err := io.Copy(tarfile, file); err != nil {
			glog.Errorf("Could not write file %s: %s", path, err)
			return err
		}
	}
	return nil
}

// ImportArchive reads from a tar Reader and writes the contents into a path
// preserving file permissions and ownership.
func ImportArchive(tarfile *tar.Reader, path string) error {
	for {
		header, err := tarfile.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			glog.Errorf("Could not import archive to %s: %s", path, err)
			return err
		}
		if err := ImportArchiveHeader(header, tarfile, path); err != nil {
			return err
		}
	}
	return nil
}

// ImportArchiveHeader imports a tarfile header to a particular path
func ImportArchiveHeader(header *tar.Header, reader io.Reader, path string) error {
	filename := filepath.Join(path, header.Name)
	switch header.Typeflag {
	case tar.TypeDir:
		if err := os.MkdirAll(filename, 0755); err != nil {
			glog.Errorf("Could not create directory at %s: %s", filename, err)
			return err
		}
	case tar.TypeSymlink:
		if err := os.Symlink(filename, header.Linkname); err != nil {
			glog.Errorf("Could not create symlink at %s: %s", filename, err)
			return err
		}
	case tar.TypeReg:
		err := func() error {
			writer, err := os.Create(filename)
			if err != nil {
				glog.Errorf("Could not create file at %s: %s", filename, err)
				return err
			}
			defer writer.Close()
			if _, err := io.Copy(writer, reader); err != nil {
				glog.Errorf("Could not copy file %s: %s", filename, err)
				return err
			}
			return nil
		}()
		if err != nil {
			return err
		}
	default:
		glog.Errorf("Found unxepected file type %b: will not import %s", header.Typeflag, filename)
		return nil
	}
	if err := os.Chown(filename, header.Uid, header.Gid); err != nil {
		glog.Warningf("Could not change file ownership for %s: %s", filename, err)
	}
	if err := os.Chmod(filename, header.FileInfo().Mode()); err != nil {
		glog.Warningf("Could not set permissions for file %s: %s", filename, err)
	}
	return nil
}

func getHeader(name, link string, fstat os.FileInfo) (*tar.Header, error) {
	header, err := tar.FileInfoHeader(fstat, link)
	if err != nil {
		return nil, err
	}
	header.Name = name
	header.Uid = int(fstat.Sys().(*syscall.Stat_t).Uid)
	header.Gid = int(fstat.Sys().(*syscall.Stat_t).Gid)
	header.ModTime = fstat.ModTime()
	return header, nil
}
