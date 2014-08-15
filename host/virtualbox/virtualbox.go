package virtualbox

import (
	"bufio"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"time"

	"log"

	"path"
	"path/filepath"
	"strconv"
	"strings"

	gossh "code.google.com/p/go.crypto/ssh"
	"github.com/boot2docker/boot2docker-cli/driver"
	"github.com/nitrous-io/hostbuilder/host"
	"github.com/nitrous-io/hostbuilder/pkg/ssh"
	"github.com/riobard/go-virtualbox"
)

var (
	confDir string
	vmdkImg string

	minSSHPort uint = 65100
	maxSSHPort uint = 65500

	// host-only config
	// the same settings as boot2docker vm so this will share the same network
	// with boot2docker
	dhcpEnabled = true                           // enable host-only network
	hostIP      = net.ParseIP("192.168.60.3")    // host-only ip address
	netmask     = ParseIPv4Mask("255.255.255.0") // host-only network DHCP
	dhcpIP      = net.ParseIP("192.168.60.99")   // DHCP server address
	dhcpLowerIP = net.ParseIP("192.168.60.103")  // DHCP lower bound
	dhcpUpperIP = net.ParseIP("192.168.60.254")  // DHCP upper bound
)

func init() {
	confDir = path.Join(os.Getenv("HOME"), ".hostbuilder")
	if err := os.MkdirAll(confDir, 0755); err != nil {
		panic(fmt.Sprintf("failed to make configuration dir %v", err))
	}

	// FIXME: do not hard-code this, but now we get it from a prepared VM
	// we should build the image from packer or sth and distribute it together
	// with the provider
	vmdkImgName := "test-box-f7b60a07c6045a4c.vmdk"
	vmdkImg = path.Join(confDir, vmdkImgName)

}

// Flag names in lowercases to be consistent with VBoxManage options.
const (
	F_acpi Flag = 1 << iota
	F_ioapic
	F_rtcuseutc
	F_cpuhotplug
	F_pae
	F_longmode
	F_synthcpu
	F_hpet
	F_hwvirtex
	F_triplefaultreset
	F_nestedpaging
	F_largepages
	F_vtxvpid
	F_vtxux
	F_accelerate3d
)

type Flag int

// Convert bool to "on"/"off"
func bool2string(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// Test if flag is set. Return "on" or "off".
func (f Flag) Get(o Flag) string {
	return bool2string(f&o == o)
}

// Machine information.
type Machine struct {
	Name       string
	UUID       string
	State      virtualbox.MachineState
	CPUs       uint
	Memory     uint // main memory (in MB)
	VRAM       uint // video memory (in MB)
	CfgFile    string
	BaseFolder string
	OSType     string
	Flag       Flag
	BootOrder  []string // max 4 slots, each in {none|floppy|dvd|disk|net}
	SSHPort    uint
}

// Modify changes the settings of the machine.
func (m *Machine) Modify() error {
	args := []string{"modifyvm", m.Name,
		"--firmware", "bios",
		"--bioslogofadein", "off",
		"--bioslogofadeout", "off",
		"--natdnshostresolver1", "on",
		"--bioslogodisplaytime", "0",
		"--biosbootmenu", "disabled",

		"--ostype", m.OSType,
		"--cpus", fmt.Sprintf("%d", m.CPUs),
		"--memory", fmt.Sprintf("%d", m.Memory),
		"--vram", fmt.Sprintf("%d", m.VRAM),

		"--acpi", m.Flag.Get(F_acpi),
		"--ioapic", m.Flag.Get(F_ioapic),
		"--rtcuseutc", m.Flag.Get(F_rtcuseutc),
		"--cpuhotplug", m.Flag.Get(F_cpuhotplug),
		"--pae", m.Flag.Get(F_pae),
		"--longmode", m.Flag.Get(F_longmode),
		"--synthcpu", m.Flag.Get(F_synthcpu),
		"--hpet", m.Flag.Get(F_hpet),
		"--hwvirtex", m.Flag.Get(F_hwvirtex),
		"--triplefaultreset", m.Flag.Get(F_triplefaultreset),
		"--nestedpaging", m.Flag.Get(F_nestedpaging),
		"--largepages", m.Flag.Get(F_largepages),
		"--vtxvpid", m.Flag.Get(F_vtxvpid),
		"--vtxux", m.Flag.Get(F_vtxux),
		"--accelerate3d", m.Flag.Get(F_accelerate3d),
	}

	for i, dev := range m.BootOrder {
		if i > 3 {
			break // Only four slots `--boot{1,2,3,4}`. Ignore the rest.
		}
		args = append(args, fmt.Sprintf("--boot%d", i+1), dev)
	}
	if err := vbm(args...); err != nil {
		return err
	}
	return m.Refresh()
}

// Refresh reloads the machine information.
func (m *Machine) Refresh() error {
	id := m.Name
	if id == "" {
		id = m.UUID
	}
	mm, err := getMachine(id)
	if err != nil {
		return err
	}
	*m = *mm
	return nil
}

// AddNATPF adds a NAT port forarding rule to the n-th NIC with the given name.
func (m *Machine) AddNATPF(n int, name string, rule driver.PFRule) error {
	return vbm("controlvm", m.Name, fmt.Sprintf("natpf%d", n),
		fmt.Sprintf("%s,%s", name, rule.Format()))
}

// DelNATPF deletes the NAT port forwarding rule with the given name from the n-th NIC.
func (m *Machine) DelNATPF(n int, name string) error {
	return vbm("controlvm", m.Name, fmt.Sprintf("natpf%d", n), "delete", name)
}

// SetNIC set the n-th NIC.
func (m *Machine) SetNIC(n int, nic NIC) error {
	args := []string{"modifyvm", m.Name,
		fmt.Sprintf("--nic%d", n), string(nic.Network),
		fmt.Sprintf("--nictype%d", n), string(nic.Hardware),
		fmt.Sprintf("--cableconnected%d", n), "on",
	}

	if nic.Network == "hostonly" {
		args = append(args, fmt.Sprintf("--hostonlyadapter%d", n), nic.HostonlyAdapter)
	}
	return vbm(args...)
}

// AddStorageCtl adds a storage controller with the given name.
func (m *Machine) AddStorageCtl(name string, ctl StorageController) error {
	args := []string{"storagectl", m.Name, "--name", name}
	if ctl.SysBus != "" {
		args = append(args, "--add", string(ctl.SysBus))
	}
	if ctl.Ports > 0 {
		args = append(args, "--portcount", fmt.Sprintf("%d", ctl.Ports))
	}
	if ctl.Chipset != "" {
		args = append(args, "--controller", string(ctl.Chipset))
	}
	args = append(args, "--hostiocache", bool2string(ctl.HostIOCache))
	args = append(args, "--bootable", bool2string(ctl.Bootable))
	return vbm(args...)
}

// DelStorageCtl deletes the storage controller with the given name.
func (m *Machine) DelStorageCtl(name string) error {
	return vbm("storagectl", m.Name, "--name", name, "--remove")
}

// AttachStorage attaches a storage medium to the named storage controller.
func (m *Machine) AttachStorage(ctlName string, medium StorageMedium) error {
	return vbm("storageattach", m.Name, "--storagectl", ctlName,
		"--port", fmt.Sprintf("%d", medium.Port),
		"--device", fmt.Sprintf("%d", medium.Device),
		"--type", string(medium.DriveType),
		"--medium", medium.Medium,
	)
}

// Start starts the machine.
func (m *Machine) Start() error {
	switch m.State {
	case virtualbox.Paused:
		return vbm("controlvm", m.Name, "resume")
	case virtualbox.Poweroff, virtualbox.Saved, virtualbox.Aborted:
		return vbm("startvm", m.Name, "--type", "headless")
	}
	if err := m.Refresh(); err == nil {
		if m.State != virtualbox.Running {
			return fmt.Errorf("Failed to start", m.Name)
		}
	}
	return nil
}

// Suspend suspends the machine and saves its state to disk.
func (m *Machine) Save() error {
	switch m.State {
	case virtualbox.Paused:
		if err := m.Start(); err != nil {
			return err
		}
	case virtualbox.Poweroff, virtualbox.Aborted, virtualbox.Saved:
		return nil
	}
	return vbm("controlvm", m.Name, "savestate")
}

// Pause pauses the execution of the machine.
func (m *Machine) Pause() error {
	switch m.State {
	case virtualbox.Paused, virtualbox.Poweroff, virtualbox.Aborted, virtualbox.Saved:
		return nil
	}
	return vbm("controlvm", m.Name, "pause")
}

// Stop gracefully stops the machine.
func (m *Machine) Stop() error {
	switch m.State {
	case virtualbox.Poweroff, virtualbox.Aborted, virtualbox.Saved:
		return nil
	case virtualbox.Paused:
		if err := m.Start(); err != nil {
			return err
		}
	}

	for m.State != virtualbox.Poweroff { // busy wait until the machine is stopped
		if err := vbm("controlvm", m.Name, "acpipowerbutton"); err != nil {
			return err
		}
		time.Sleep(1 * time.Second)
		if err := m.Refresh(); err != nil {
			return err
		}
	}
	return nil
}

// Poweroff forcefully stops the machine. State is lost and might corrupt the disk image.
func (m *Machine) Poweroff() error {
	switch m.State {
	case virtualbox.Poweroff, virtualbox.Aborted, virtualbox.Saved:
		return nil
	}
	return vbm("controlvm", m.Name, "poweroff")
}

// Restart gracefully restarts the machine.
func (m *Machine) Restart() error {
	switch m.State {
	case virtualbox.Paused, virtualbox.Saved:
		if err := m.Start(); err != nil {
			return err
		}
	}
	if err := m.Stop(); err != nil {
		return err
	}
	return m.Start()
}

// Reset forcefully restarts the machine. State is lost and might corrupt the disk image.
func (m *Machine) Reset() error {
	switch m.State {
	case virtualbox.Paused, virtualbox.Saved:
		if err := m.Start(); err != nil {
			return err
		}
	}
	return vbm("controlvm", m.Name, "reset")
}

// Delete deletes the machine and associated disk images.
func (m *Machine) Delete() error {
	if err := m.Poweroff(); err != nil {
		return err
	}
	return vbm("unregistervm", m.Name, "--delete")
}

type HostConfig struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	CPUs       uint     `json:"cpus"`
	Memory     uint     `json:"memory"` // in MB
	PublicKeys []string `json:"public_keys"`

	// for setting up repository on the host
	AptUsername string `json:"apt_username"` // username of nitrous.io
	AptPassword string `json:"apt_password"` // nitrous.io access token
	AptLocation string `json:"apt_location"` // URI of the repository
	AptDist     string `json:"apt_dist"`     // distribution of the repo
}

type Host struct {
	Config *HostConfig

	machine    *virtualbox.Machine
	privateKey *rsa.PrivateKey
	conn       *ssh.Conn
}

func (h *Host) Create() (*host.Info, error) {
	m, err := createMachine(h.Config)
	if err != nil {
		log.Printf("failed to create machine %s", err)
		return nil, err
	}

	if err := m.Start(); err != nil {
		log.Printf("Failed to start machine %q: %s", m, err)
		return nil, err
	}

	log.Println("Waiting for VM to be started...")
	//give the VM a little time to start
	time.Sleep(600 * time.Millisecond)

	h.Config.ID = m.UUID

	return h.Info()
}

func (h *Host) Destroy() error {
	m, err := getMachine(h.Config.ID)
	if err != nil {
		return err
	}
	return m.Delete()
}

func (h *Host) Info() (*host.Info, error) {
	m, err := getMachine(h.Config.ID)
	if err != nil {
		return nil, err
	}
	ipAddress := ""
	for i := 1; i < 30; i++ {
		var err error
		ipAddress, err = requestIP(m.SSHPort)
		log.Println("check", ipAddress, err)
		if ipAddress != "" {
			break
		}
	}
	if ipAddress == "" {
		return nil, fmt.Errorf("failed to detect ip address of the container")
	}
	info := &host.Info{
		Provider:       "virtualbox",
		ID:             m.UUID,
		IPAddress:      ipAddress,
		FreighterdPort: 65333,
	}
	return info, nil
}

func (h *Host) Stop() (*host.Info, error) {
	m, err := getMachine(h.Config.ID)
	if err != nil {
		return nil, err
	}
	if err := m.Stop(); err != nil {
		return nil, err
	}
	return h.Info()
}

func (h *Host) Start() (*host.Info, error) {
	m, err := getMachine(h.Config.ID)
	if err != nil {
		return nil, err
	}
	if err := m.Start(); err != nil {
		return nil, err
	}
	return h.Info()
}

func (h *Host) Restart() (*host.Info, error) {
	m, err := getMachine(h.Config.ID)
	if err != nil {
		return nil, err
	}
	if err := m.Restart(); err != nil {
		return nil, err
	}
	return h.Info()
}

func NewHost(defaultConfig map[string]string, config string) (host.Host, error) {
	hostConfig := &HostConfig{}
	if err := json.Unmarshal([]byte(config), hostConfig); err != nil {
		log.Printf("virtualbox: invalid config %s", config)
		return nil, err
	}
	host := &Host{
		Config: hostConfig,
	}
	return host, nil
}

func init() {
	host.Hosts["virtualbox"] = NewHost
}

func createMachine(cfg *HostConfig) (*Machine, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("machine name is empty: %+v", cfg)
	}

	// Check if a machine with the given name already exists.
	machineNames, err := listMachines()
	if err != nil {
		return nil, err
	}
	for _, name := range machineNames {
		if name == cfg.Name {
			return nil, ErrMachineExist
		}
	}

	// Create and register the machine.
	args := []string{"createvm", "--name", cfg.Name, "--register"}
	if err := vbm(args...); err != nil {
		return nil, err
	}

	m, err := getMachine(cfg.Name)
	if err != nil {
		return nil, err
	}

	// Configure VM
	setExtra(cfg.Name, "VBoxInternal/CPUM/EnableHVP", "1")
	m.OSType = "Linux26_64"
	m.CPUs = uint(cfg.CPUs)
	m.Memory = cfg.Memory

	m.Flag |= F_pae
	m.Flag |= F_longmode // important: use x86-64 processor
	m.Flag |= F_rtcuseutc
	m.Flag |= F_acpi
	m.Flag |= F_ioapic
	m.Flag |= F_hpet
	m.Flag |= F_hwvirtex
	m.Flag |= F_vtxvpid
	m.Flag |= F_largepages
	m.Flag |= F_nestedpaging

	// Set VM boot order
	m.BootOrder = []string{"dvd"}
	if err := m.Modify(); err != nil {
		return m, err
	}

	// Set NIC #1 to use NAT
	m.SetNIC(1, NIC{Network: NICNetNAT, Hardware: VirtIO})

	openPort, err := getOpenPort()
	if err != nil {
		return m, err
	}
	pfRules := map[string]driver.PFRule{
		"ssh": {Proto: driver.PFTCP, HostIP: net.ParseIP("127.0.0.1"), HostPort: uint16(openPort), GuestPort: 22},
	}
	for name, rule := range pfRules {
		if err := m.AddNATPF(1, name, rule); err != nil {
			return m, err
		}
	}

	hostIFName, err := getHostOnlyNetworkInterface(cfg)
	if err != nil {
		return m, err
	}

	// Set NIC #2 to use host-only
	if err := m.SetNIC(2, NIC{Network: NICNetHostonly, Hardware: VirtIO, HostonlyAdapter: hostIFName}); err != nil {
		return m, err
	}

	// Set VM storage
	if err := m.AddStorageCtl("SATA", StorageController{SysBus: SysBusSATA, HostIOCache: true, Bootable: true}); err != nil {
		log.Printf("failed to add storage control %v", err)
		return m, err
	}

	diskImg := filepath.Join(confDir, fmt.Sprintf("%s.vmdk", cfg.Name))
	if _, err := os.Stat(diskImg); err != nil {
		if !os.IsNotExist(err) {
			return m, err
		}

		if err := copyDiskImage(vmdkImg, diskImg); err != nil {
			log.Printf("failed to copy disk %v", err)
			return m, err
		}
	}

	if err := m.AttachStorage("SATA", StorageMedium{Port: 1, Device: 0, DriveType: DriveHDD, Medium: diskImg}); err != nil {
		log.Printf("failed to attach storage %v", err)
		return m, err
	}

	return m, nil
}

// ListMachines lists all registered machines' name.
// We assume that our machine's name is unique and it's reasonable because we
// use slug as machine name
func listMachines() ([]string, error) {
	out, err := vbmOut("list", "vms")
	if err != nil {
		return nil, err
	}
	ms := []string{}
	s := bufio.NewScanner(strings.NewReader(out))
	for s.Scan() {
		res := reVMNameUUID.FindStringSubmatch(s.Text())
		if res == nil {
			continue
		}
		ms = append(ms, res[1])
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return ms, nil
}

// GetMachine finds a machine by its name or UUID.
func getMachine(id string) (*Machine, error) {
	stdout, stderr, err := vbmOutErr("showvminfo", id, "--machinereadable")
	if err != nil {
		if reMachineNotFound.FindString(stderr) != "" {
			return nil, ErrMachineNotExist
		}
		return nil, err
	}
	s := bufio.NewScanner(strings.NewReader(stdout))
	m := &Machine{}
	for s.Scan() {
		res := reVMInfoLine.FindStringSubmatch(s.Text())
		if res == nil {
			continue
		}
		key := res[1]
		if key == "" {
			key = res[2]
		}
		val := res[3]
		if val == "" {
			val = res[4]
		}

		switch key {
		case "name":
			m.Name = val
		case "UUID":
			m.UUID = val
		case "VMState":
			m.State = virtualbox.MachineState(val)
		case "memory":
			n, err := strconv.ParseUint(val, 10, 32)
			if err != nil {
				return nil, err
			}
			m.Memory = uint(n)
		case "cpus":
			n, err := strconv.ParseUint(val, 10, 32)
			if err != nil {
				return nil, err
			}
			m.CPUs = uint(n)
		case "vram":
			n, err := strconv.ParseUint(val, 10, 32)
			if err != nil {
				return nil, err
			}
			m.VRAM = uint(n)
		case "CfgFile":
			m.CfgFile = val
			m.BaseFolder = filepath.Dir(val)
		default:
			if strings.HasPrefix(key, "Forwarding(") {
				vals := strings.Split(val, ",")
				n, err := strconv.ParseUint(vals[3], 10, 32)
				if err != nil {
					return nil, err
				}
				switch vals[0] {
				case "ssh":
					m.SSHPort = uint(n)
				}
			}
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return m, nil
}

// setExtra sets extra data. Name could be "global"|<uuid>|<vmname>
func setExtra(name, key, val string) error {
	return vbm("setextradata", name, key, val)
}

// delExtra deletes extra data. Name could be "global"|<uuid>|<vmname>
func delExtra(name, key string) error {
	return vbm("setextradata", name, key)
}

// Get or create the hostonly network interface
func getHostOnlyNetworkInterface(cfg *HostConfig) (string, error) {
	// Check if the interface/dhcp exists.
	nets, err := HostonlyNets()
	if err != nil {
		return "", err
	}

	dhcps, err := DHCPs()
	if err != nil {
		return "", err
	}

	for _, n := range nets {
		if dhcp, ok := dhcps[n.NetworkName]; ok {
			if dhcp.IPv4.IP.Equal(dhcpIP) &&
				dhcp.IPv4.Mask.String() == netmask.String() &&
				dhcp.LowerIP.Equal(dhcpLowerIP) &&
				dhcp.UpperIP.Equal(dhcpUpperIP) &&
				dhcp.Enabled == dhcpEnabled {
				return n.Name, nil
			}
		}
	}

	// No existing host-only interface found. Create a new one.
	hostonlyNet, err := CreateHostonlyNet()
	if err != nil {
		return "", err
	}
	hostonlyNet.IPv4.IP = hostIP
	hostonlyNet.IPv4.Mask = netmask
	if err := hostonlyNet.Config(); err != nil {
		return "", err
	}

	// Create and add a DHCP server to the host-only network
	dhcp := DHCP{}
	dhcp.IPv4.IP = dhcpIP
	dhcp.IPv4.Mask = netmask
	dhcp.LowerIP = dhcpLowerIP
	dhcp.UpperIP = dhcpUpperIP
	dhcp.Enabled = true
	if err := AddHostonlyDHCP(hostonlyNet.Name, dhcp); err != nil {
		return "", err
	}
	return hostonlyNet.Name, nil
}

func reader(r io.Reader) {
	buf := make([]byte, 1024)
	for {
		_, err := io.ReadAtLeast(r, buf[:], 20)
		if err != nil {
			return
		}
	}
}

// request IP from SSH
func requestIP(sshPort uint) (string, error) {
	config := &gossh.ClientConfig{
		User: "vagrant",
		Auth: []gossh.AuthMethod{
			gossh.Password("vagrant"),
		},
	}
	c, err := gossh.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", sshPort), config)
	if err != nil {
		return "", fmt.Errorf("failed to dial: %s", err)
	}
	defer c.Close()
	s, err := c.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session %s", err)
		log.Fatalf("unable to create session: %s", err)
	}
	out, err := s.CombinedOutput("ip addr show dev eth1")
	if err != nil {
		return "", fmt.Errorf("failed to execute ip command %s", err)
	}
	cmdOutput := string(out)
	log.Println("SSH returned: ", cmdOutput)

	IP := ""
	// parse to find: inet 192.168.59.103/24 brd 192.168.59.255 scope global eth1
	lines := strings.Split(cmdOutput, "\n")
	for _, line := range lines {
		vals := strings.Split(strings.TrimSpace(line), " ")
		if len(vals) >= 2 && vals[0] == "inet" {
			IP = vals[1][:strings.Index(vals[1], "/")]
			break
		}
	}
	return IP, nil
}

// getOpenPort returns a free port on the host
func getOpenPort() (uint, error) {
	var offset uint = 0
	portRange := int(maxSSHPort - minSSHPort)
	if portRange > 0 {
		// Have to check if > 0 to avoid a panic
		offset = uint(rand.Intn(portRange))
	}

	var sshHostPort uint
	for {
		sshHostPort = offset + minSSHPort
		log.Printf("Trying port: %d", sshHostPort)
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", sshHostPort))
		if err == nil {
			defer l.Close()
			break
		}
	}
	if sshHostPort == 0 {
		return 0, fmt.Errorf("failed to find available SSH port between %d and %d", minSSHPort, maxSSHPort)
	}
	return sshHostPort, nil
}

// Copy disk image from given source path to destination
func copyDiskImage(src, dst string) (err error) {
	return vbm("clonehd", src, dst)
}
