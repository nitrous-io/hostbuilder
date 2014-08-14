package dummy

import (
	"encoding/json"
	"errors"

	"github.com/nitrous-io/hostbuilder/host"
)

type Host struct {
	ID             string `json:"id"`
	IPAddress      string `json:"ip_address"`
	DestroySuccess bool   `json:"destroy"`
	CreateSuccess  bool   `json:"create"`
	StopSuccess    bool   `json:"stop"`
	StartSuccess   bool   `json:"start"`
	RestartSuccess bool   `json:"restart"`
}

func (h *Host) Create() (*host.Info, error) {
	if !h.CreateSuccess {
		return nil, errors.New("failed to create host")
	}
	return h.Info()
}

func (h *Host) Destroy() error {
	if h.DestroySuccess {
		return nil
	}
	return errors.New("Failed to destroy host")
}

func (h *Host) Info() (*host.Info, error) {
	info := &host.Info{
		Provider:       "dummy",
		ID:             h.ID,
		IPAddress:      h.IPAddress,
		FreighterdPort: uint16(65333),
	}
	return info, nil
}

func (h *Host) Stop() error {
	if h.StopSuccess {
		return nil
	}
	return errors.New("Failed to stop host")
}

func (h *Host) Start() error {
	if h.StartSuccess {
		return nil
	}
	return errors.New("Failed to start host")
}

func (h *Host) Restart() error {
	if h.RestartSuccess {
		return nil
	}
	return errors.New("Failed to restart host")
}

func (h *Host) Availability() (map[string][]string, error) {
	return map[string][]string{
		"dummy": {"small", "medium"},
	}, nil
}

func NewHost(defaultConfig map[string]string, config string) (host.Host, error) {
	var host Host
	json.Unmarshal([]byte(config), &host)
	return &host, nil
}

func init() {
	host.Hosts["dummy"] = NewHost
}
