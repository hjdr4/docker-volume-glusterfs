package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/ddliu/go-httpclient"
	"github.com/docker/go-plugins-helpers/volume"
)

const (
	volumesPath      = "/api/1.0/volumes"
	volumeGetPath    = "/api/1.0/volume/%s"
	volumeCreatePath = "/api/1.0/volume/%s"
	volumeRemovePath = "/api/1.0/volume/%s"
	volumeStopPath   = "/api/1.0/volume/%s/stop"
)

type peer struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type rvolume struct {
	Name       string `json:"name"`
	UUID       string `json:"uuid"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	NumBricks  int    `json:"num_bricks"`
	Distribute int    `json:"distribute"`
	Stripe     int    `json:"stripe"`
	Replica    int    `json:"replica"`
	Transport  string `json:"transport"`
}

type response struct {
	Ok  bool   `json:"ok"`
	Err string `json:"error,omitempty"`
}

type peerResponse struct {
	Data []peer `json:"data,omitempty"`
	response
}

type volumesResponse struct {
	Data []rvolume `json:"data,omitempty"`
	response
}

func responseCheck(resp *http.Response) error {
	var p response
	err := json.NewDecoder(resp.Body).Decode(&p)
	if err != nil {
		return err
	}

	if !p.Ok {
		return fmt.Errorf(p.Err)
	}

	return nil
}

func responseCheckHttpClient(resp *httpclient.Response) error {
	var p response
	err := json.NewDecoder(resp.Body).Decode(&p)
	if err != nil {
		return err
	}

	if !p.Ok {
		return fmt.Errorf(p.Err)
	}

	return nil
}

//-------------------------------

type volumeName struct {
	name        string
	connections int
}

type GlusterRestClient struct {
	servers   []string
	urlPrefix string
	port      int
	base      string
}

//NewGlusterRestClient constructor
func NewGlusterRestClient(servers []string, login, password string, port int, base string) GlusterRestClient {
	client := GlusterRestClient{
		servers:   servers,
		urlPrefix: fmt.Sprintf("http://%s:%s@", login, password),
		port:      port,
		base:      base}
	return client
}

func (g GlusterRestClient) GetAddress(server string, url string) string {
	return fmt.Sprintf("%s%s:%s%s", g.urlPrefix, server, strconv.Itoa(g.port), url)
}

func (g GlusterRestClient) Get(url string) (*http.Response, error) {
	for _, server := range g.servers {
		res, err := http.Get(g.GetAddress(server, url))
		if err != nil {
			continue
		}
		return res, err
	}
	return nil, fmt.Errorf("Get(): could not diag with any server")
}

func (g GlusterRestClient) PostForm(url string, params url.Values) (*http.Response, error) {
	for _, server := range g.servers {
		address := g.GetAddress(server, url)
		res, err := http.PostForm(address, params)
		if err != nil {
			continue
		}
		return res, err
	}
	return nil, fmt.Errorf("PostForm(): could not diag with any server")
}

func (g GlusterRestClient) Delete(url string, params url.Values) (*httpclient.Response, error) {
	for _, server := range g.servers {
		client := httpclient.WithHeader("Content-Type", "application/x-www-form-urlencoded")
		body := strings.NewReader(params.Encode())
		res, err := client.Do("DELETE", g.GetAddress(server, url), nil, body)

		if err != nil {
			continue
		}

		return res, nil
	}

	return nil, fmt.Errorf("Delete(): could not diag with any server")
}

func (c GlusterRestClient) listVolumes() ([]string, error) {
	res, err := c.Get(volumesPath)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var d volumesResponse
	if err := json.NewDecoder(res.Body).Decode(&d); err != nil {
		return nil, err
	}

	if !d.Ok {
		return nil, fmt.Errorf(d.Err)
	}

	var ret []string
	for _, volume := range d.Data {
		ret = append(ret, volume.Name)
	}
	return ret, nil
}

func (c GlusterRestClient) volumeExists(name string) (bool, error) {
	res, err := c.Get(fmt.Sprintf(volumeGetPath, name))
	if err != nil {
		return false, err
	}

	defer res.Body.Close()

	var d volumesResponse
	if err := json.NewDecoder(res.Body).Decode(&d); err != nil {
		return false, err
	}
	if !d.Ok {
		return false, nil
	}
	return true, nil
}

type glusterfsDriver struct {
	root    string
	servers []string
	volumes map[string]*volumeName
	m       *sync.Mutex
	client  GlusterRestClient
}

func (c GlusterRestClient) deleteVolume(name string) error {
	params := url.Values{"stop": {"true"}}
	url := fmt.Sprintf(volumeRemovePath, name)
	resp, err := c.Delete(url, params)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	checked := responseCheckHttpClient(resp)
	if checked != nil {
		return checked
	}

	return nil
}

func (c GlusterRestClient) createVolume(name string) error {
	bricks := make([]string, len(c.servers))
	for i, p := range c.servers {
		bricks[i] = fmt.Sprintf("%s:%s", p, filepath.Join(c.base, name))
	}

	params := url.Values{
		"bricks":    {strings.Join(bricks, ",")},
		"transport": {"tcp"},
		"start":     {"true"},
		"force":     {"true"},
	}

	if len(c.servers) > 1 {
		params.Add("replica", strconv.Itoa(len(c.servers)))
	}

	resp, err := c.PostForm(fmt.Sprintf(volumeCreatePath, name), params)
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
