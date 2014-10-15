package digitalocean

import (
	"bytes"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"code.google.com/p/goauth2/oauth"

	"github.com/digitalocean/godo"
	"github.com/nitrous-io/hostbuilder/context"
	"github.com/nitrous-io/hostbuilder/host"
	"github.com/nitrous-io/hostbuilder/pkg/ssh"
)

var (
	TLSCALocation   = "/etc/freighterd/ca.crt"
	TLSCertLocation = "/etc/freighterd/cert.crt"
	TLSKeyLocation  = "/etc/freighterd/key.crt"
	SecretLocation  = "/etc/freighterd/secret"

	StateWaitTime         = 3 * time.Second
	FreighterPingTimeout  = 1 * time.Second
	FreighterPingAttempts = []time.Duration{
		5 * time.Second,
		30 * time.Second,
		90 * time.Second,
	}

	// FIXME: the map should belongs to client, not the builder
	regionsMap = map[string]string{
		"sgp": "sgp1",
		"nyc": "nyc2",
		"sfo": "sfo1",
		"ams": "ams2",
		"lon": "lon1",
	}
)

type tlsConfig struct {
	CA   string `json:"ca"`
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

type HostConfig struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	ImageID    string     `json:"image_id"`
	RegionID   string     `json:"region_id"`
	SizeID     string     `json:"size_id"`
	PublicKeys []string   `json:"public_keys"`
	TLS        *tlsConfig `json:"tls"`
	Secret     string     `json:"secret"`

	// for setting up repository on the host
	AptUsername string `json:"apt_username"` // username of nitrous.io
	AptPassword string `json:"apt_password"` // nitrous.io access token
	AptLocation string `json:"apt_location"` // URI of the repository
	AptDist     string `json:"apt_dist"`     // distribution of the repo
}

type Host struct {
	Client *godo.Client
	Config *HostConfig

	droplet    *godo.Droplet
	privateKey *rsa.PrivateKey
	conn       *ssh.Conn
	sshKey     *godo.Key
}

func (h *Host) Create(w io.Writer) (*host.Info, error) {
	defer func() {
		if h.sshKey != nil {
			fmt.Fprintf(w, "starting: remove temporary ssh key")
			_, err := h.Client.Keys.DeleteByID(h.sshKey.ID)
			if err != nil {
				fmt.Fprintf(w, "failed: remove temporary ssh key")
			}
			fmt.Fprintf(w, "finished: remove temporary ssh key")
		}
	}()
	steps := []context.Step{
		{"Creating key", createSSHKey},
		{"Creating host", createDroplet},
		{"Waiting for the host to be ready", waitForSSH},
		{"Writing key", writePublicKeys},
		{"Creating settings", createEtcFreighterd},
		{"Writing certificates", writeTLSCerts},
		{"Writing token", writeSecret},
		{"Setting up repos", writeRepository},
		{"Fetching updates", updateSoftware},
	}
	ctx := context.NewContext(h, steps, w)
	if err := ctx.Execute(); err != nil {
		if h.droplet != nil {
			fmt.Fprintf(w, "Canceling host creation...")
			if resp, derr := h.Client.Droplets.Delete(h.droplet.ID); derr != nil {
				fmt.Fprintf(w, "Canceling host creation...Failed!")
				return nil, fmt.Errorf("Failed to destroy after create error: %s, destroy error: %v, resp: %v", err, derr, resp)
			}
			fmt.Fprintf(w, "Canceling host creation...Done!")
		}
		return nil, err
	}

	info := h.dropletToInfo()
	// do not remove a host if freighter is down, we keep the host for debugging
	_ = h.pingFreighter(info, w)
	fmt.Fprintf(w, "finished: the droplet should be ready now")
	return info, nil
}

func (h *Host) pingFreighter(dropletInfo *host.Info, w io.Writer) error {
	contactAddr := fmt.Sprintf("https://%s:%d", dropletInfo.IPAddress, dropletInfo.FreighterdPort)
	fmt.Fprintf(w, "Contacting freighter at %s", contactAddr)
	if !WaitForServer(contactAddr, FreighterPingTimeout, FreighterPingAttempts) {
		fmt.Fprintf(w, "failed: freighter not available")
		return fmt.Errorf("freighter not available")
	}
	return nil
}

func (h *Host) Destroy() error {
	id, err := h.id()
	if err != nil {
		return err
	}
	dropletRoot, resp, err := h.Client.Droplets.Get(id)
	if err != nil {
		// if the droplet does not exist, then consider that it is sucessfully
		// destroyed
		// https://github.com/nitrous-io/nitro/issues/99
		if resp.StatusCode == http.StatusNotFound {
			return nil
		}
		return err
	}
	h.droplet = dropletRoot.Droplet
	if resp, err := h.Client.Droplets.Delete(id); err != nil {
		// if the droplet does not exist, then consider that it is sucessfully
		// destroyed
		// https://github.com/nitrous-io/nitro/issues/99
		if resp.StatusCode == http.StatusNotFound {
			return nil
		}
		return err
	}
	return nil
}

func (h *Host) Info() (*host.Info, error) {
	id, err := h.id()
	if err != nil {
		return nil, err
	}
	dropletRoot, _, err := h.Client.Droplets.Get(id)
	if err != nil {
		return nil, err
	}
	h.droplet = dropletRoot.Droplet
	return h.dropletToInfo(), nil
}

func (h *Host) dropletToInfo() *host.Info {
	return &host.Info{
		Provider:       "digitalocean",
		ID:             strconv.Itoa(h.droplet.ID),
		IPAddress:      h.ipAddress(),
		FreighterdPort: uint16(65333),
	}
}

func (h *Host) Stop() (*host.Info, error) {
	id, err := h.id()
	if err != nil {
		return nil, err
	}

	action, _, err := h.Client.DropletActions.PowerOff(id)
	if err != nil {
		return nil, err
	}
	_, err = WaitForAction(h.Client.Actions, action.ID, StateWaitTime, 15*time.Minute)
	if err != nil {
		return nil, err
	}

	return h.Info()
}

func (h *Host) Start() (*host.Info, error) {
	id, err := h.id()
	if err != nil {
		return nil, err
	}

	action, _, err := h.Client.DropletActions.PowerOn(id)
	if err != nil {
		return nil, err
	}
	_, err = WaitForAction(h.Client.Actions, action.ID, StateWaitTime, 15*time.Minute)
	if err != nil {
		return nil, err
	}

	info, err := h.Info()
	if err != nil {
		return nil, err
	}

	if err := h.pingFreighter(info, ioutil.Discard); err != nil {
		return nil, err
	}

	return info, nil
}

func (h *Host) Restart() (*host.Info, error) {
	id, err := h.id()
	if err != nil {
		return nil, err
	}
	action, _, err := h.Client.DropletActions.Reboot(id)
	if err != nil {
		return nil, err
	}
	_, err = WaitForAction(h.Client.Actions, action.ID, StateWaitTime, 15*time.Minute)
	if err != nil {
		return nil, err
	}

	info, err := h.Info()
	if err != nil {
		return nil, err
	}

	if err := h.pingFreighter(info, ioutil.Discard); err != nil {
		return nil, err
	}

	return info, nil
}

func (h *Host) id() (int, error) {
	id64, err := strconv.ParseInt(h.Config.ID, 10, 64)
	if err != nil {
		return 0, err
	}
	return int(id64), nil
}

// ipAddress get the ipv4 address of the droplet. DigitalOcean always returns
// one IPv4 for a valid container. The format of Networks is:
// "networks": {
//    "v4": [
//      {
//        "ip_address": "127.0.0.1",
//        "netmask": "255.255.255.0",
//        "gateway": "127.0.0.2",
//        "type": "public"
//      }
//    ],
//  }
//
//  We are not allocating/using IPv6 yet so that's empty
func (h *Host) ipAddress() string {
	return h.droplet.Networks.V4[0].IPAddress
}

func NewHost(defaultConfig map[string]string, config string) (host.Host, error) {
	hostConfig := &HostConfig{}
	if err := json.Unmarshal([]byte(config), hostConfig); err != nil {
		log.Printf("digitalocean: invalid config %s", config)
		return nil, err
	}
	if defaultConfig["token"] == "" {
		return nil, errors.New("digitalocean builder requires valid access token")
	}
	t := &oauth.Transport{
		Token: &oauth.Token{AccessToken: defaultConfig["token"]},
	}
	client := godo.NewClient(t.Client())
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
	sshKey, _, err := h.Client.Keys.Create(&godo.KeyCreateRequest{
		Name:      h.Config.Name,
		PublicKey: string(pubKey),
	})
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

	normalizedRegion, ok := regionsMap[config.RegionID]
	if !ok {
		return fmt.Errorf("Invalid region or lookup failure: '%s'", config.RegionID)
	}
	dropletRoot, _, err := h.Client.Droplets.Create(&godo.DropletCreateRequest{
		Name:   config.Name,
		Region: normalizedRegion,
		Size:   config.SizeID,
		Image:  config.ImageID,
		SSHKeys: []interface{}{
			sshKey.ID,
		},
	})
	if err != nil {
		return err
	}

	h.droplet = dropletRoot.Droplet

	if len(dropletRoot.Links.Actions) > 0 {
		la := dropletRoot.Links.Actions[0]
		_, err = WaitForAction(h.Client.Actions, la.ID, StateWaitTime, 15*time.Minute)
		if err != nil {
			return err
		}
	} else {
		log.Printf("[WARNING] digitalocean does not return action data for host %+v", hst)
		return nil
	}

	dropletRoot, _, err = h.Client.Droplets.Get(dropletRoot.Droplet.ID)
	if err != nil {
		return fmt.Errorf("create: failed to get droplet after completed %s", err)
	}

	h.droplet = dropletRoot.Droplet
	return nil
}

func waitForSSH(ctx *context.Context, hst host.Host) error {
	h := hst.(*Host)
	privateKey := h.privateKey
	// check that we can ssh into the server
	address := fmt.Sprintf("%s:%s", h.ipAddress(), "65100")
	sshConn, err := ssh.WaitForSSH(address, "root", privateKey,
		ssh.TimeWait, make(chan struct{}))
	if err != nil {
		return err
	}
	log.Printf("SSH READY for droplet %v", h.droplet.ID)
	h.conn = sshConn
	return nil
}

func writePublicKeys(ctx *context.Context, hst host.Host) error {
	h := hst.(*Host)
	pubKeyR := bytes.NewBuffer(nil)
	for _, k := range h.Config.PublicKeys {
		fmt.Fprintln(pubKeyR, k)
	}
	return writeFile(h, map[string]io.Reader{
		"/root/.ssh/authorized_keys": pubKeyR,
	})
}

func createEtcFreighterd(ctx *context.Context, hst host.Host) error {
	return hst.(*Host).conn.Execute("mkdir -p /etc/freighterd")
}

func writeTLSCerts(ctx *context.Context, hst host.Host) error {
	h := hst.(*Host)
	return writeFile(h, map[string]io.Reader{
		TLSCALocation:   bytes.NewBufferString(h.Config.TLS.CA),
		TLSCertLocation: bytes.NewBufferString(h.Config.TLS.Cert),
		TLSKeyLocation:  bytes.NewBufferString(h.Config.TLS.Key),
	})
}

func writeSecret(ctx *context.Context, hst host.Host) error {
	h := hst.(*Host)
	return writeFile(h, map[string]io.Reader{
		SecretLocation: bytes.NewBufferString(h.Config.Secret),
	})
}

func writeFile(h *Host, files map[string]io.Reader) error {
	flist := []string{}
	for dest, val := range files {
		if err := h.conn.Upload(dest, val); err != nil {
			return err
		}
		flist = append(flist, dest)
	}
	log.Printf("Uploaded (%s) to %v", strings.Join(flist, " "), h.ipAddress())
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
	log.Printf("Uploaded apt details at %v", h.ipAddress())
	return nil
}

func updateSoftware(ctx *context.Context, hst host.Host) error {
	h := hst.(*Host)
	updateSoftwareCmd :=
		`apt-get update && apt-get install -yq freighterd && service freighterd restart`
	if err := h.conn.Execute(updateSoftwareCmd); err != nil {
		return err
	}
	log.Printf("Updated software on %v", h.ipAddress())
	return nil
}
