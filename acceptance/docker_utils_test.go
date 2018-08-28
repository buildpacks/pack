package acceptance_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"testing"
)

type DockerDaemon struct {
	client *http.Client
	url    func(path string, query map[string]string) string
}

func NewDockerDaemon() *DockerDaemon {
	return &DockerDaemon{
		client: &http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", "/var/run/docker.sock")
				},
			},
		},
		url: func(path string, query map[string]string) string {
			u := url.URL{Scheme: "http", Host: "unix", Path: path}
			q := u.Query()
			for k, v := range query {
				q.Set(k, v)
			}
			u.RawQuery = q.Encode()
			return u.String()
		},
	}
}

func (d *DockerDaemon) Do(method, path string, query map[string]string, data interface{}) (io.ReadCloser, http.Header, error) {
	// fmt.Println("DOCKER:", method, path, query, data)
	var postData io.Reader
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return nil, nil, err
		}
		postData = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, d.url(path, query), postData)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := d.client.Do(req)
	if err != nil {
		return nil, nil, err
	}

	if res.StatusCode >= 300 {
		defer res.Body.Close()
		if res.StatusCode == 404 {
			io.Copy(ioutil.Discard, res.Body)
			return nil, nil, fmt.Errorf("404 Not Found: %s: %s", method, path)
		}
		var out struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			return nil, nil, err
		}
		return nil, nil, errors.New(out.Message)
	}

	return res.Body, res.Header, nil
}

type ImageInspect struct {
	ID     string `json:"Id"`
	Config struct {
		Entrypoint []string
		Cmd        []string
		Env        []string
		Labels     map[string]string
		WorkingDir string
		Users      string
	}
}

func (d *DockerDaemon) InspectImage(t *testing.T, name string) ImageInspect {
	t.Helper()
	body, _, err := d.Do("GET", fmt.Sprintf("/images/%s/json", name), nil, nil)
	if err != nil {
		t.Fatalf("image: %s: %s", name, err)
	}
	defer body.Close()

	var out ImageInspect
	if err := json.NewDecoder(body).Decode(&out); err != nil {
		t.Fatalf("parsing json: %s: %s", name, err)
	}

	return out
}

func (d *DockerDaemon) CreateContainer(t *testing.T, imageName string) string {
	t.Helper()
	body, _, err := d.Do("POST", "/containers/create", nil, map[string]string{"Image": imageName})
	if err != nil {
		t.Fatalf("create container: %s: %s", imageName, err)
	}
	defer body.Close()

	var out struct {
		ID string `json:"Id"`
	}
	if err := json.NewDecoder(body).Decode(&out); err != nil {
		t.Fatalf("parsing json for: %s: %s", imageName, err)
	}

	return out.ID
}

type ContainerFileInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

func (d *DockerDaemon) FileFromImage(t *testing.T, imageName, path string) (out ContainerFileInfo) {
	t.Helper()
	containerID := d.CreateContainer(t, imageName)
	defer d.RemoveContainer(t, containerID)

	body, header, err := d.Do("HEAD", fmt.Sprintf("/containers/%s/archive", containerID), map[string]string{"path": path}, nil)
	if err != nil {
		t.Fatalf("file info: %s: %s", imageName, err)
	}
	defer body.Close()

	decoded, err := base64.StdEncoding.DecodeString(header.Get("X-Docker-Container-Path-Stat"))
	if err != nil {
		t.Fatalf("decode file info: %s: %s", imageName, err)
	}

	if err := json.Unmarshal(decoded, &out); err != nil {
		t.Fatalf("parsing json for: %s: %s", imageName, err)
	}
	return out
}

func (d *DockerDaemon) Pull(t *testing.T, imageName, tag string) {
	t.Helper()
	body, _, err := d.Do("POST", "/images/create", map[string]string{"fromImage": imageName, "tag": tag}, nil)
	if err != nil {
		t.Fatalf("docker pull: %s: %s", imageName, err)
	}
	defer body.Close()
	out := make(map[string]interface{})
	for {
		if err := json.NewDecoder(body).Decode(&out); err != nil {
			break
		}
		// fmt.Printf("OUT: %#v\n", out)
		if out["message"] != nil {
			t.Fatal(out["message"])
		}
	}
}

func (d *DockerDaemon) RemoveContainer(t *testing.T, containerID string) {
	t.Helper()
	body, _, err := d.Do("DELETE", fmt.Sprintf("/containers/%s", containerID), nil, nil)
	if err != nil {
		t.Fatalf("docker rm: %s: %s", containerID, err)
	}
	defer body.Close()
	io.Copy(ioutil.Discard, body)
}

func (d *DockerDaemon) Kill(containerIDs ...string) error {
	for _, containerID := range containerIDs {
		body, _, err := d.Do("POST", fmt.Sprintf("/containers/%s/kill", containerID), nil, nil)
		if err != nil {
			return fmt.Errorf("docker kill: %s: %s", containerID, err)
		}
		defer body.Close()
		io.Copy(ioutil.Discard, body)
	}
	return nil
}

func (d *DockerDaemon) RemoveImage(names ...string) error {
	for _, name := range names {
		body, _, err := d.Do("DELETE", fmt.Sprintf("/images/%s", name), map[string]string{"force": "true"}, nil)
		if err != nil {
			return fmt.Errorf("docker rmi: %s: %s", name, err)
		}
		defer body.Close()
		io.Copy(ioutil.Discard, body)
	}
	return nil
}

type DockerRegistry struct {
	client *http.Client
}

func NewDockerRegistry() *DockerRegistry {
	return &DockerRegistry{
		client: &http.Client{},
	}
}

type RegistryImageInspect struct {
	ID     string `json:"ID"`
	Config struct {
		Entrypoint []string
		Cmd        []string
		Env        []string
		Labels     map[string]string
		WorkingDir string
		Users      string
	} `json:"config"`
}

func (d *DockerRegistry) InspectImage(t *testing.T, name string) ImageInspect {
	t.Helper()
	u, err := url.Parse("http://" + name)
	if err != nil {
		t.Fatalf("parse name: %s: %s", name, err)
	}
	u.Path = "/v2" + u.Path + "/manifests/latest"

	res, err := d.client.Get(u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode >= 300 {
		defer res.Body.Close()
		if res.StatusCode == 404 {
			io.Copy(ioutil.Discard, res.Body)
			t.Fatalf("404 Not Found: %s", name)
		}
		var out struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		t.Fatal(out.Message)
	}

	var out1 struct {
		History []struct {
			V1Compat string `json:"v1Compatibility"`
		} `json:"history"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out1); err != nil {
		t.Fatalf("parsing json 1: %s: %s", name, err)
	}
	var out2 RegistryImageInspect
	if err := json.Unmarshal([]byte(out1.History[0].V1Compat), &out2); err != nil {
		t.Fatalf("parsing json 2: %s: %s", name, err)
	}

	return ImageInspect(out2)
}
