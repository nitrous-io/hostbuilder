package digitalocean

import (
	"bytes"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/nitrous-io/hostbuilder/context"
	"github.com/nitrous-io/hostbuilder/host"
	do "github.com/nitrous-io/hostbuilder/pkg/digitalocean"
	"github.com/nitrous-io/hostbuilder/pkg/ssh"
)

type HostConfig struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	ImageID    string   `json:"image_id"`
	RegionID   string   `json:"region_id"`
	SizeID     string   `json:"size_id"`
	PublicKeys []string `json:"public_keys"`

	// for setting up repository on the host
	AptUsername string `json:"apt_username"` // username of nitrous.io
	AptPassword string `json:"apt_password"` // nitrous.io access token
	AptLocation string `json:"apt_location"` // URI of the repository
	AptDist     string `json:"apt_dist"`     // distribution of the repo
}

type Host struct {
	Client *do.Client
	Config *HostConfig

	instance   *do.Droplet
	privateKey *rsa.PrivateKey
	conn       *ssh.Conn
	sshKey     *do.SSHKey
}

func (h *Host) Create() (*host.Info, error) {
	defer func() {
		if h.sshKey != nil {
			h.Client.DestroyKey(h.sshKey.ID)
		}
	}()
	steps := []context.Step{
		createSSHKey,
		createDroplet,
		waitForSSH,
		writePublicKeys,
		writeRepository,
	}
	ctx := context.NewContext(h, steps)
	if err := ctx.Execute(); err != nil {
		if h.instance != nil {
			if derr := h.Client.DestroyDroplet(h.instance.ID); derr != nil {
				return nil, fmt.Errorf("Failed to destroy after create error: %v, %v", err, derr)
			}
		}
		return nil, err
	}
	return h.Info()
}

func (h *Host) Destroy() error {
	id, err := h.id()
	if err != nil {
		return err
	}
	droplet, err := h.Client.DropletInfo(id)
	if err != nil {
		return err
	}
	h.instance = droplet
	if err := h.Client.DestroyDroplet(id); err != nil {
		return err
	}
	return nil
}

func (h *Host) Info() (*host.Info, error) {
	if h.instance == nil {
		return nil, errors.New("Invalid ec2 instance")
	}
	info := &host.Info{
		Provider:       "digitalocean",
		ID:             strconv.Itoa(h.instance.ID),
		IPAddress:      h.instance.IpAddress,
		FreighterdPort: uint16(65333),
	}
	return info, nil
}

func (h *Host) Stop() error {
	id, err := h.id()
	if err != nil {
		return err
	}
	if err := h.Client.PowerOffDroplet(id); err != nil {
		return err
	}
	return nil
}

func (h *Host) Start() error {
	id, err := h.id()
	if err != nil {
		return err
	}
	if err := h.Client.PowerOnDroplet(id); err != nil {
		return err
	}
	return nil
}

func (h *Host) Restart() error {
	id, err := h.id()
	if err != nil {
		return err
	}
	if err := h.Client.RebootDroplet(id); err != nil {
		return err
	}
	return nil
}

func (h *Host) Availability() (map[string][]string, error) {
	return nil, nil
}

func (h *Host) id() (int, error) {
	id64, err := strconv.ParseInt(h.Config.ID, 10, 64)
	if err != nil {
		return 0, err
	}
	return int(id64), nil
}

func NewHost(defaultConfig map[string]string, config string) (host.Host, error) {
	client := do.NewClient(defaultConfig["client_id"], defaultConfig["api_key"])
	hostConfig := &HostConfig{}
	if err := json.Unmarshal([]byte(config), hostConfig); err != nil {
		log.Printf("digitalocean: invalid config %s", config)
		return nil, err
	}
	host := &Host{
		Config: hostConfig,
		Client: client,
	}
	return host, nil
}

func init() {
	host.Hosts["digitalocean"] = NewHost
}

func createSSHKey(ctx *context.Context, hst host.Host) error {
	h := hst.(*Host)
	// create a tmp keypair
	privateKey, err := ssh.GenerateSSHKey()
	if err != nil {
		return err
	}
	h.privateKey = privateKey
	pubKey, err := ssh.GetPubKey(privateKey)
	if err != nil {
		return err
	}
	sshKey, err := h.Client.CreateKey(h.Config.Name, string(pubKey))
	if err != nil {
		return err
	}
	h.sshKey = sshKey
	return nil
}

func createDroplet(ctx *context.Context, hst host.Host) error {
	h := hst.(*Host)
	config := h.Config
	sshKey := h.sshKey
	var err error

	// create the droplet
	h.instance, err = h.Client.CreateDroplet(config.Name, config.SizeID,
		config.ImageID, config.RegionID, sshKey.ID, false)
	if err != nil {
		return err
	}

	// wait for the droplet to be active
	droplet, err := do.WaitForState("active", h.instance.ID, h.Client,
		do.StateWaitTime, 5*time.Minute)
	if err != nil {
		return err
	}
	h.instance = droplet
	return nil
}

func waitForSSH(ctx *context.Context, hst host.Host) error {
	h := hst.(*Host)
	privateKey := h.privateKey
	// check that we can ssh into the server
	address := fmt.Sprintf("%s:%s", h.instance.IpAddress, "65100")
	sshConn, err := ssh.WaitForSSH(address, "root", privateKey,
		ssh.TimeWait, make(chan struct{}))
	if err != nil {
		return err
	}
	log.Println("SSH READY for droplet %v", h.instance.ID)
	h.conn = sshConn
	return nil
}

func writePublicKeys(ctx *context.Context, hst host.Host) error {
	h := hst.(*Host)
	pubKeyR := bytes.NewBuffer(nil)
	for _, k := range h.Config.PublicKeys {
		fmt.Fprintln(pubKeyR, k)
	}
	if err := h.conn.Upload("/root/.ssh/authorized_keys", pubKeyR); err != nil {
		return err
	}
	log.Printf("Uploaded authorized keys at %v", h.instance.IpAddress)
	return nil
}

func writeRepository(ctx *context.Context, hst host.Host) error {
	h := hst.(*Host)
	sourceData := bytes.NewBuffer(nil)
	if err := host.SourceTemplate.Execute(sourceData, h.Config); err != nil {
		return err
	}
	if err := h.conn.Upload(host.SourceLocation, sourceData); err != nil {
		return err
	}
	log.Printf("Uploaded apt details at %v", h.instance.IpAddress)
	return nil
}
