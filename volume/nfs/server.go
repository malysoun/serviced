package nfs

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/control-center/serviced/utils"
	"github.com/control-center/serviced/volume"
	"github.com/zenoss/glog"
)

var (
	AllowedDaemons = []string{"rpcbind", "mountd", "nfsd", "statd", "lockd", "rquotad"}
)

var (
	ErrPathIsNotDir         = errors.New("path is not a directory")
	ErrInvalidExportName    = errors.New("invalid export name")
	ErrVolumeShareExists    = errors.New("volume share exists")
	ErrVolumeShareNotExists = errors.New("volume share does not exist")
)

type NetworkFileShareServer interface {
	GetShares() (names []string)
	GetShare(name string) (path string)
	AddShare(path, name string) error
	RemoveShare(name string) error
	EnableShare(name string) error
	DisableShare(name string) error
}

type NFSServer struct {
	Systemd
	root    string
	network string
	options []string
	volumes map[string]string
	allows  map[string]struct{}
}

func NewNFSServer(exportPath, network string, options []string) (*NFSServer, error) {
	// verify the exportPath exists and is valid
	if !filepath.IsAbs(exportPath) {
		return nil, volume.ErrPathIsNotAbs
	}
	// clean up any current exports
	volumes := make(map[string]string)
	fis, err := ioutil.ReadDir(exportPath)
	if err != nil {
		return nil, err
	}
	for _, fi := range fis {
		if fi.IsDir() {
			volumePath := filepath.Join(exportPath, fi.Name())
			if mountpath, err := GetMount(volumePath); err != nil {
				return nil, err
			} else if mountpath != "" {
				volumes[fi.Name()] = mountpath
			}
		}
	}
	// verify the network
	if network == "0.0.0.0/0" {
		network = "*"
	} else if _, _, err := net.ParseCIDR(network); err != nil {
		return nil, err
	}
	server := &NFSServer{
		Systemd: NewSystemdService(NewService("nfs-server", "nfs-kernel-server"), utils.Platform),
		root:    exportPath,
		network: network,
		options: options,
		volumes: volumes,
		allows:  make(map[string]struct{}),
	}
	if err := server.Sync(); err != nil {
		return nil, err
	}
	return server, nil
}

func (server *NFSServer) Allow(client string) error {
	server.allows[client] = struct{}{}
	return server.syncHostsAllow()
}

func (server *NFSServer) Revoke(client string) error {
	if _, exists := server.allows[client]; exists {
		delete(server.allows, client)
		return server.syncHostsAllow()
	}
	return nil
}

func (server *NFSServer) GetShares() []string {
	volumes := []string{}
	for name := range server.volumes {
		volumes = append(volumes, name)
	}
	return volumes
}

func (server *NFSServer) GetShare(name string) string {
	return server.volumes[name]
}

func (server *NFSServer) AddShare(name, path string) error {
	if _, exists := server.volumes[name]; exists {
		return ErrVolumeShareExists
	}
	// create the path
	exportVolume := filepath.Join(server.root, name)
	if err := os.MkdirAll(exportVolume, 0755); os.IsExist(err) {
		// unmount just in case
		syscall.Unmount(exportVolume, syscall.MNT_DETACH)
	} else if err != nil {
		return err
	}
	mount := exec.Command("mount", "--bind", path, exportVolume)
	if output, err := mount.CombinedOutput(); err != nil {
		glog.Errorf("Could not bind mount %s at %s: %s (%s)", path, name, output, err)
		return err
	}
	server.volumes[name] = path
	return server.syncExports()
}

func (server *NFSServer) RemoveShare(name string) error {
	if _, exists := server.volumes[name]; !exists {
		return ErrVolumeShareNotExists
	}
	if err := server.DisableShare(name); err != nil {
		return err
	}
	delete(server.volumes, name)
	exportVolume := filepath.Join(server.root, name)
	if err := os.RemoveAll(exportVolume); err != nil {
		glog.Warningf("Could not remove volume %s: %s", name, err)
	}
	return server.syncExports()
}

func (server *NFSServer) EnableShare(name string) error {
	path, exists := server.volumes[name]
	if !exists {
		return ErrVolumeShareNotExists
	}
	exportVolume := filepath.Join(server.root, name)
	mount := exec.Command("mount", "--bind", path, exportVolume)
	if output, err := mount.CombinedOutput(); err != nil {
		glog.Errorf("Could not bind mount %s at %s: %s (%s)", path, name, output, err)
		return err
	}
	exportfs := exec.Command("exportfs", "-o", strings.Join(server.options, ","), fmt.Sprintf("%s:%s", server.network, exportVolume))
	if output, err := exportfs.CombinedOutput(); err != nil {
		glog.Errorf("Could not enable volume %s: %s (%s)", name, output, err)
		return err
	}
	return nil
}

func (server *NFSServer) DisableShare(name string) error {
	if _, exists := server.volumes[name]; !exists {
		return ErrVolumeShareNotExists
	}
	exportVolume := filepath.Join(server.root, name)
	exportfs := exec.Command("exportfs", "-u", fmt.Sprintf("%s:%s", server.network, exportVolume))
	if output, err := exportfs.CombinedOutput(); err != nil {
		glog.Errorf("Could not disable volume %s: %s (%s)", name, output, err)
		return err
	}
	if err := syscall.Unmount(exportVolume, syscall.MNT_DETACH); err != nil {
		return err
	}
	return nil
}

func (server *NFSServer) Sync() error {
	// sync clients
	clients := []string{}
	for allow := range server.allows {
		clients = append(clients, allow)
	}
	if err := server.syncHostsAllow(); err != nil {
		return err
	}
	if err := server.syncHostsDeny(); err != nil {
		return err
	}
	if err := server.syncExports(); err != nil {
		return err
	}
	return nil
}

func (server *NFSServer) syncHostsAllow() error {
	clients := []string{}
	for client := range server.allows {
		clients = append(clients, client)
	}
	return AtomicSync("/etc/hosts.allow", func(reader io.Reader, writer io.Writer) error {
		return SyncHosts(clients, reader, writer)
	})
}

func (server *NFSServer) syncHostsDeny() error {
	return AtomicSync("/etc/hosts.deny", func(reader io.Reader, writer io.Writer) error {
		return SyncHosts([]string{"ALL"}, reader, writer)
	})
}

func (server *NFSServer) syncExports() error {
	err := AtomicSync("/etc/exports", func(reader io.Reader, writer io.Writer) error {
		return SyncExports(server.root, server.GetShares(), server.network, server.options, reader, writer)
	})
	if err != nil {
		return err
	}
	export := exec.Command("exportfs", "-ra")
	if output, err := export.CombinedOutput(); err != nil {
		glog.Errorf("Could not export: %s (%s)", output, err)
		return err
	}
	return nil
}

func scan(header, footer []byte) bufio.SplitFunc {
	var seekStart, seekEnd bool
	return func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if !seekStart {
			// find header
			if index := bytes.Index(data, header); index >= 0 {
				seekStart = true
				return index, data[:index], nil
			} else if !atEOF {
				return 0, nil, nil
			} else {
				return len(data), data, nil
			}
		} else if !seekEnd {
			// find the body
			start := 0
			for {
				advance, token, err = bufio.ScanLines(data[start:], atEOF)
				if err == nil && token != nil {
					if bytes.Equal(bytes.TrimSpace(token), footer) {
						seekEnd = true
						return start + advance, data[:start+advance], nil
					}
				} else if atEOF {
					return len(data), data, nil
				} else {
					return advance, token, err
				}
			}
		} else {
			// get the footer
			if atEOF {
				return len(data), data, nil
			} else {
				return 0, nil, nil
			}
		}
	}
}

func GetMount(dest string) (string, error) {
	mountData, err := ioutil.ReadFile("/etc/mtab")
	if err != nil {
		glog.Errorf("Could not check mount on %s: %s (%s)", dest, err)
		return "", err
	}
	re := regexp.MustCompile(fmt.Sprintf("(.*) %s", dest))
	if matches := re.FindSubmatch(mountData); matches != nil {
		return string(matches[1]), nil
	}
	return "", nil
}

func SyncExports(root string, volumes []string, network string, permissions []string, reader io.Reader, writer io.Writer) error {
	scanner := bufio.NewScanner(reader)
	// set up a splitter to extract the serviced configuration
	scanner.Split(scan([]byte("# --- SERVICED EXPORTS BEGIN ---"), []byte("# --- SERVICED EXPORTS END ---")))
	output := ""

	// scan the header
	if scanner.Scan() {
		output += scanner.Text()
	} else {
		return errors.New("could not scan header")
	}
	// scan the body
	scanner.Scan()
	output += fmt.Sprintln("# --- SERVICED EXPORTS BEGIN ---")
	output += fmt.Sprintf("%s %s(fsid=0,%s)\n", root, network, strings.Join(permissions, ","))
	for _, volume := range volumes {
		output += fmt.Sprintf("%s %s(%s)\n", filepath.Join(root, volume), network, strings.Join(permissions, ","))
	}
	output += fmt.Sprintln("# --- SERVICED EXPORTS END ---")
	// scan the footer
	if scanner.Scan() {
		output += scanner.Text()
	}
	if _, err := fmt.Fprint(writer, output); err != nil {
		return err
	}
	return nil
}

func SyncHosts(clients []string, reader io.Reader, writer io.Writer) error {
	scanner := bufio.NewScanner(reader)
	// set up a splitter to extract the serviced configuration
	scanner.Split(scan([]byte("# serviced"), []byte("")))
	output := ""
	// scan the header
	if scanner.Scan() {
		output += scanner.Text()
	} else {
		return errors.New("could not scan header")
	}
	// scan the body
	output += fmt.Sprintln("# serviced, do not remove")
	output += fmt.Sprintf("%s : %s\n\n", strings.Join(AllowedDaemons, " "), strings.Join(clients, " "))
	// scan the footer
	if scanner.Scan() {
		output += scanner.Text()
	}
	if _, err := fmt.Fprint(writer, output); err != nil {
		return err
	}
	return nil
}

func AtomicSync(filename string, sync func(reader io.Reader, writer io.Writer) error) error {
	tempfilename, err := func() (string, error) {
		file, err := os.Open(filename)
		if err != nil {
			return "", err
		}
		defer file.Close()
		tempfile, err := ioutil.TempFile("", "atomicsync-")
		if err != nil {
			return "", err
		}
		defer func() {
			tempfile.Close()
			if err != nil {
				os.RemoveAll(tempfile.Name())
			}
		}()
		if err := sync(file, tempfile); err != nil {
			return "", err
		}
		return tempfile.Name(), err
	}()
	if err != nil {
		return err
	}
	if err := os.Rename(tempfilename, filename); err != nil {
		os.RemoveAll(tempfilename)
		return err
	}
	return nil
}
