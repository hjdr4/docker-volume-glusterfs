package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	httpclient "github.com/ddliu/go-httpclient"
)

//Model
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

func responseCheckHTTPClient(resp *httpclient.Response) error {
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

//GetAddress returns the full url to contact
func (g GlusterRestClient) GetAddress(server string, url string) string {
	return fmt.Sprintf("%s%s:%s%s", g.urlPrefix, server, strconv.Itoa(g.port), url)
}

//Get tries to HTTP GET on all servers
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

//PostForm tries to POST with application/x-www-form-urlencoded format
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

//Delete tries to DELETE with application/x-www-form-urlencoded format
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

func (g GlusterRestClient) listVolumes() ([]string, error) {
	res, err := g.Get(volumesPath)
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

func (g GlusterRestClient) volumeExists(name string) (bool, error) {
	res, err := g.Get(fmt.Sprintf(volumeGetPath, name))
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

//GlusterRestClient struct
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
