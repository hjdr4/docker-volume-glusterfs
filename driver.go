package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/docker/go-plugins-helpers/volume"
)

type volumeName struct {
	name        string
	connections int
}

type glusterfsDriver struct {
	root    string
	servers []string
	volumes map[string]*volumeName
	m       *sync.Mutex
	client  GlusterRestClient
}

func (g GlusterRestClient) deleteVolume(name string) error {
	params := url.Values{"stop": {"true"}}
	url := fmt.Sprintf(volumeRemovePath, name)
	resp, err := g.Delete(url, params)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	checked := responseCheckHTTPClient(resp)
	if checked != nil {
		return checked
	}

	return nil
}

func (g GlusterRestClient) createVolume(name string) error {
	bricks := make([]string, len(g.servers))
	for i, p := range g.servers {
		bricks[i] = fmt.Sprintf("%s:%s", p, filepath.Join(g.base, name))
	}

	params := url.Values{
		"bricks":    {strings.Join(bricks, ",")},
		"transport": {"tcp"},
		"start":     {"true"},
		"force":     {"true"},
	}

	if len(g.servers) > 1 {
		params.Add("replica", strconv.Itoa(len(g.servers)))
	}

	resp, err := g.PostForm(fmt.Sprintf(volumeCreatePath, name), params)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	checked := responseCheck(resp)
	if checked != nil {
		return checked
	}

	return nil
}

func newGlusterfsDriver(mountPath string, servers []string, login string, password string, port int, base string) glusterfsDriver {
	d := glusterfsDriver{
		root:    mountPath,
		servers: servers,
		volumes: map[string]*volumeName{},
		m:       &sync.Mutex{},
		client:  NewGlusterRestClient(servers, login, password, port, base)}

	return d
}

func (d glusterfsDriver) Create(r volume.Request) volume.Response {
	err := d.client.createVolume(r.Name)
	if err != nil {
		return volume.Response{Err: err.Error()}
	}
	return volume.Response{}
}

func (d glusterfsDriver) Remove(r volume.Request) volume.Response {
	err := d.client.deleteVolume(r.Name)
	if err != nil {
		return volume.Response{
			Err: err.Error()}
	}
	return volume.Response{}
}

func (d glusterfsDriver) Path(r volume.Request) volume.Response {
	return volume.Response{Mountpoint: d.mountpoint(r.Name)}
}

func (d glusterfsDriver) Mount(r volume.MountRequest) volume.Response {
	d.m.Lock()
	defer d.m.Unlock()
	m := d.mountpoint(r.Name)
	log.Printf("Mounting volume %s on %s\n", r.Name, m)

	s, ok := d.volumes[m]
	if ok && s.connections > 0 {
		s.connections++
		return volume.Response{Mountpoint: m}
	}

	fi, err := os.Lstat(m)

	if os.IsNotExist(err) {
		if err := os.MkdirAll(m, 0755); err != nil {
			return volume.Response{Err: err.Error()}
		}
	} else if err != nil {
		return volume.Response{Err: err.Error()}
	}

	if fi != nil && !fi.IsDir() {
		return volume.Response{Err: fmt.Sprintf("%v already exist and it's not a directory", m)}
	}

	if err := d.mountVolume(r.Name, m); err != nil {
		return volume.Response{Err: err.Error()}
	}

	d.volumes[m] = &volumeName{name: r.Name, connections: 1}

	return volume.Response{Mountpoint: m}
}

func (d glusterfsDriver) Unmount(r volume.UnmountRequest) volume.Response {
	d.m.Lock()
	defer d.m.Unlock()
	m := d.mountpoint(r.Name)
	log.Printf("Unmounting volume %s from %s\n", r.Name, m)

	if s, ok := d.volumes[m]; ok {
		if s.connections == 1 {
			if err := d.unmountVolume(m); err != nil {
				return volume.Response{Err: err.Error()}
			}
		}
		s.connections--
	} else {
		return volume.Response{Err: fmt.Sprintf("Unable to find volume mounted on %s", m)}
	}

	return volume.Response{}
}

func (d glusterfsDriver) Get(r volume.Request) volume.Response {
	d.m.Lock()
	defer d.m.Unlock()
	m := d.mountpoint(r.Name)

	exists, err := d.client.volumeExists(r.Name)
	if err != nil {
		return volume.Response{Err: err.Error()}
	}
	if exists {
		return volume.Response{Volume: &volume.Volume{Name: r.Name, Mountpoint: m}}
	}
	return volume.Response{Err: "Volume " + r.Name + " does not exist"}
}

func (d glusterfsDriver) List(r volume.Request) volume.Response {
	d.m.Lock()
	defer d.m.Unlock()

	names, err := d.client.listVolumes()
	if err != nil {
		return volume.Response{Err: err.Error()}
	}
	var vols []*volume.Volume
	for _, name := range names {
		vol := &volume.Volume{Name: name, Mountpoint: d.mountpoint(name)}
		vols = append(vols, vol)
	}
	return volume.Response{Volumes: vols}
}

func bytesToLines(bytes []byte) []string {
	raw := string(bytes[:])
	lines := strings.Split(raw, "\n")
	return lines
}

func (d *glusterfsDriver) mountpoint(name string) string {
	return filepath.Join(d.root, name)
}

func (d *glusterfsDriver) mountVolume(name, destination string) error {
	var serverNodes []string
	for _, server := range d.servers {
		serverNodes = append(serverNodes, fmt.Sprintf("-s %s", server))
	}

	cmd := fmt.Sprintf("glusterfs --volfile-id=%s %s %s", name, strings.Join(serverNodes[:], " "), destination)
	if out, err := exec.Command("sh", "-c", cmd).CombinedOutput(); err != nil {
		log.Println(string(out))
		return err
	}
	return nil
}

func (d *glusterfsDriver) unmountVolume(target string) error {
	cmd := fmt.Sprintf("umount %s", target)
	if out, err := exec.Command("sh", "-c", cmd).CombinedOutput(); err != nil {
		log.Println(string(out))
		return err
	}
	return nil
}

func (d glusterfsDriver) Capabilities(r volume.Request) volume.Response {
	var res volume.Response
	res.Capabilities = volume.Capability{Scope: "global"}
	return res
}
