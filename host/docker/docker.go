package docker

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	dockerclient "github.com/fsouza/go-dockerclient"
	"github.com/nitrous-io/hostbuilder/host"
)

var (
	TLSCALocation   = "/etc/freighterd/ca.crt"
	TLSCertLocation = "/etc/freighterd/cert.crt"
	TLSKeyLocation  = "/etc/freighterd/key.crt"
	SecretLocation  = "/etc/freighterd/secret"
)

type tlsConfig struct {
	CA   string `json:"ca"`
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

type Host struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	PublicKeys         []string   `json:"public_keys"`
	IpAddress          string     `json:"ip_address"`
	FreighterdLocation string     `json:"freighterd_location"`
	TLS                *tlsConfig `json:"tls"`
	Secret             string     `json:"secret"`

	client *dockerclient.Client
}

// Create creates the docker container which acts as a container host, and write the
// progress into w
func (h *Host) Create(w io.Writer) (*host.Info, error) {
	w = io.MultiWriter(os.Stdout, w)
	for i, k := range h.PublicKeys {
		h.PublicKeys[i] = strings.TrimSpace(k)
	}
	fmt.Fprintf(w, "Creating host image")
	imageName, err := h.createImage()
	if err != nil {
		fmt.Fprintln(w, "Failed to create host image")
		return nil, err
	}
	fmt.Fprintf(w, "Created host image")
	fmt.Fprintf(w, "Creating host with image “%s”", imageName)
	c, err := h.createContainer(imageName)
	if err != nil {
		fmt.Fprintf(w, "Failed to create host")
		return nil, err
	}
	fmt.Fprintf(w, "Host created")
	return c, nil
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
		IPAddress:      "127.0.0.1",
		FreighterdPort: uint16(getFreighterdPort(container)),
	}, nil
}

func (h *Host) Stop() (*host.Info, error) {
	err := h.client.StopContainer(h.ID, uint(5))
	if err != nil {
		return nil, err
	}

	return h.Info()
}

func (h *Host) Start() (*host.Info, error) {
	err := h.client.StartContainer(h.ID, &dockerclient.HostConfig{
		Privileged:      true,
		PublishAllPorts: true,
		Binds:           []string{fmt.Sprintf("%s:/freighterd", h.FreighterdLocation)},
	})

	if err != nil {
		return nil, err
	}

	return h.Info()
}

func (h *Host) Restart() (*host.Info, error) {
	if err := h.client.StopContainer(h.ID, uint(5)); err != nil {
		return nil, err
	}

	hostConfig := &dockerclient.HostConfig{
		Privileged:      true,
		PublishAllPorts: true,
		Binds:           []string{fmt.Sprintf("%s:/freighterd", h.FreighterdLocation)},
	}
	if err := h.client.StartContainer(h.ID, hostConfig); err != nil {
		return nil, err
	}

	return h.Info()
}

func (h *Host) createImage() (string, error) {
	buf := bytes.NewBuffer(nil)
	if err := dockerfileTemplate.Execute(buf, h); err != nil {
		return "", err
	}

	t := time.Now()
	inputbuf := bytes.NewBuffer(nil)
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
		OutputStream: os.Stdout,
	}
	log.Printf("docker: building image %s", imageName)
	if err := h.client.BuildImage(opts); err != nil {
		return "", err
	}
	return imageName, nil
}

func (h *Host) createContainer(imageName string) (*host.Info, error) {
	log.Printf("docker: creating container from image %+s", imageName)
	container, err := h.client.CreateContainer(dockerclient.CreateContainerOptions{
		Name: h.Name,
		Config: &dockerclient.Config{
			Hostname: h.Name,
			Image:    imageName,
			Cmd:      []string{"/start-freighterd"},
		},
	})
	if err != nil {
		return nil, err
	}
	h.ID = container.ID
	info, err := h.Start()
	if err != nil {
		return nil, err
	}
	return info, nil
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
