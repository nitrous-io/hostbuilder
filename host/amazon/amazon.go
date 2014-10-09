package amazon

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"text/template"

	"github.com/mitchellh/goamz/ec2"
	"github.com/nitrous-io/hostbuilder/host"
)

var userDataTemplate = `#cloud-config
{{if .PublicKeys}}
ssh_authorized_keys:
{{range .PublicKeys}}
	- {{.}}
{{end}}
{{end}}
`
var compiledUserData *template.Template

type HostConfig struct {
	ID               string            `json:"id"`
	SourceAMI        string            `json:"source_ami"`
	AvailabilityZone string            `json:"availability_zone"`
	InstanceType     string            `json:"instance_type"`
	RawRegion        string            `json:"region"`
	PublicKeys       []string          `json:"public_keys"`
	RawSecurityGroup map[string]string `json:"security_group"`
}

func (h *HostConfig) generateUserData() ([]byte, error) {
	userData := bytes.NewBufferString("")
	if err := compiledUserData.Execute(userData, h); err != nil {
		return []byte{}, err
	}
	return userData.Bytes(), nil
}

func (h *HostConfig) securityGroups() []ec2.SecurityGroup {
	return []ec2.SecurityGroup{
		ec2.SecurityGroup{
			Id:   h.RawSecurityGroup["id"],
			Name: h.RawSecurityGroup["name"],
		},
	}
}

type Host struct {
	AccessConfig *AccessConfig
	Config       *HostConfig
	provider     *ec2.EC2

	instance *ec2.Instance
}

func (h *Host) Create(w io.Writer) (*host.Info, error) {
	w = io.MultiWriter(os.Stdout, w)
	// generate key pair
	k, err := generateTempKeypair(h.provider)
	if err != nil {
		return nil, err
	}
	defer k.Delete()

	// generate user data
	userData, err := h.Config.generateUserData()
	if err != nil {
		return nil, err
	}
	runOpts := &ec2.RunInstances{
		KeyName:        k.name,
		ImageId:        h.Config.SourceAMI,
		InstanceType:   h.Config.InstanceType,
		SecurityGroups: h.Config.securityGroups(),
		AvailZone:      h.Config.AvailabilityZone,
		UserData:       userData,
		MinCount:       0,
		MaxCount:       0,
	}
	// TODO: check if source ami is valid and exists

	// create the instance now
	resp, err := h.provider.RunInstances(runOpts)
	if err != nil {
		log.Printf("amazon: failed to create new instance %v", err)
		return nil, err
	}
	h.instance = &resp.Instances[0]

	// wait for the instance to be ready
	state := &StateChangeConf{
		Pending: []string{"pending"},
		Target:  "running",
		Refresh: InstanceStateRefreshFunc(h.provider, h.instance),
	}
	updatedInstance, err := WaitForState(state)
	if err != nil {
		return nil, err
	}
	h.instance = updatedInstance.(*ec2.Instance)
	return h.Info()
}

func (h *Host) Destroy() error {
	var instancesResp *ec2.InstancesResp
	var err error
	if instancesResp, err = h.provider.Instances([]string{h.Config.ID}, nil); err != nil {
		return err
	}
	if _, err := h.provider.TerminateInstances([]string{h.Config.ID}); err != nil {
		return err
	}
	instance := instancesResp.Reservations[0].Instances[0]
	state := StateChangeConf{
		Pending: []string{"pending", "running", "shutting-down", "stopped", "stopping"},
		Refresh: InstanceStateRefreshFunc(h.provider, &instance),
		Target:  "terminated",
	}
	updatedInstance, err := WaitForState(&state)
	if err != nil {
		return err
	}
	h.instance = updatedInstance.(*ec2.Instance)
	return nil
}

func (h *Host) Info() (*host.Info, error) {
	if h.instance == nil {
		return nil, errors.New("Invalid ec2 instance")
	}
	info := &host.Info{
		Provider:  "amazon",
		ID:        h.instance.InstanceId,
		IPAddress: h.instance.PublicIpAddress,
	}
	return info, nil
}

func (h *Host) Stop() (*host.Info, error) {
	panic("amazon provider does not support stop yet")
}

func (h *Host) Start() (*host.Info, error) {
	panic("amazon provider does not support start yet")
}

func (h *Host) Restart() (*host.Info, error) {
	panic("amazon provider does not support restart yet")
}

func NewHost(defaultConfig map[string]string, config string) (host.Host, error) {
	hostConfig := &HostConfig{}
	if err := json.Unmarshal([]byte(config), hostConfig); err != nil {
		log.Printf("amazon: invalid config %s", config)
		return nil, err
	}
	accessConfig := &AccessConfig{
		AccessKey: defaultConfig["access_key"],
		SecretKey: defaultConfig["secret_key"],
		RawRegion: hostConfig.RawRegion,
	}
	auth, err := accessConfig.Auth()
	if err != nil {
		log.Printf("amazon: auth failed: %v", err)
		return nil, err
	}
	region, err := accessConfig.Region()
	if err != nil {
		log.Printf("amazon: invalid region: %v", err)
		return nil, err
	}
	host := &Host{
		AccessConfig: accessConfig,
		Config:       hostConfig,
		provider:     ec2.New(auth, region),
	}
	return host, nil
}

func init() {
	compiledUserData = template.Must(template.New("userData").Parse(userDataTemplate))
	host.Hosts["amazon"] = NewHost
}
