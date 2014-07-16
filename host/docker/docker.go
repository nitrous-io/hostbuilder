package docker

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	dockerclient "github.com/fsouza/go-dockerclient"
	"github.com/nitrous-io/hostbuilder/host"
)

type Host struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	PublicKeys         []string `json:"public_keys"`
	IpAddress          string   `json:"ip_address"`
	FreighterdLocation string   `json:"freighterd_location"`

	client *dockerclient.Client
}

func (h *Host) Create() (*host.Info, error) {
	imageName, err := h.createImage()
	if err != nil {
		return nil, err
	}
	containerID, err := h.createContainer(imageName)
	if err != nil {
		return nil, err
	}
	h.ID = containerID
	return h.Info()
}

func (h *Host) Destroy() error {
	return h.client.RemoveContainer(dockerclient.RemoveContainerOptions{
		ID:    h.ID,
		Force: true,
	})
}

func (h *Host) Info() (*host.Info, error) {
	container, err := h.client.InspectContainer(h.ID)
	if err != nil {
		return nil, err
	}

	return &host.Info{
		Provider:       "docker",
		ID:             h.ID,
		IPAddress:      container.NetworkSettings.IPAddress,
		FreighterdPort: uint16(getFreighterdPort(container)),
	}, nil
}

func (h *Host) Stop() error {
	return h.client.KillContainer(dockerclient.KillContainerOptions{
		ID: h.ID,
	})
}

func (h *Host) Start() error {
	return h.client.StartContainer(h.ID, &dockerclient.HostConfig{
		Privileged: true,
	})
}

func (h *Host) Restart() error {
	return h.client.RestartContainer(h.ID, uint(5))
}

func (h *Host) Availability() (map[string][]string, error) {
	return nil, nil
}

func (h *Host) createImage() (string, error) {
	buf := bytes.NewBuffer(nil)
	if err := dockerfileTemplate.Execute(buf, h); err != nil {
		return "", err
	}

	t := time.Now()
	inputbuf, outputbuf := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	tr := tar.NewWriter(inputbuf)
	tr.WriteHeader(&tar.Header{
		Name:       "Dockerfile",
		Size:       int64(buf.Len()),
		ModTime:    t,
		AccessTime: t,
		ChangeTime: t,
	})
	tr.Write(buf.Bytes())
	tr.Close()

	imageName := "registry.nitrous.io/" + h.Name
	opts := dockerclient.BuildImageOptions{
		Name:         imageName,
		InputStream:  inputbuf,
		OutputStream: outputbuf,
	}
	if err := h.client.BuildImage(opts); err != nil {
		return "", err
	}
	return imageName, nil
}

func (h *Host) createContainer(imageName string) (string, error) {
	container, err := h.client.CreateContainer(dockerclient.CreateContainerOptions{
		Name:   h.Name,
		Config: &dockerclient.Config{Hostname: h.Name, Image: imageName, Cmd: []string{"/start-freighterd"}},
	})
	if err != nil {
		return "", err
	}
	// we want privileged because this will be a dind container
	err = h.client.StartContainer(container.ID, &dockerclient.HostConfig{
		Privileged:      true,
		PublishAllPorts: true,
		Binds:           []string{fmt.Sprintf("%s:/freighterd", h.FreighterdLocation)},
	})
	if err != nil {
		return "", err
	}
	return container.ID, nil
}

func NewHost(defaultConfig map[string]string, config string) (host.Host, error) {
	host := &Host{}
	if err := json.Unmarshal([]byte(config), host); err != nil {
		return nil, err
	}
	// fetch docker client endpoint from defaultConfig
	endpoint, ok := defaultConfig["endpoint"]
	if !ok {
		endpoint = "unix:///var/run/docker.sock"
	}
	freighterdLocation, _ := defaultConfig["freighterd_location"]
	host.FreighterdLocation = freighterdLocation

	client, err := dockerclient.NewClient(endpoint)
	if err != nil {
		return nil, err
	}
	host.client = client

	return host, nil
}

func getFreighterdPort(container *dockerclient.Container) uint16 {
	var freighterdPort int = 0
	var err error

	freighterdPortMappings := container.NetworkSettings.Ports["65333/tcp"]
	if len(freighterdPortMappings) == 1 {
		freighterdPort, err = strconv.Atoi(freighterdPortMappings[0].HostPort)
		if err != nil {
			freighterdPort = 0
		}
	} else {
		freighterdPort = 0
	}
	return uint16(freighterdPort)
}

func init() {
	host.Hosts["docker"] = NewHost
}
