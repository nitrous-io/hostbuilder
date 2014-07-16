package host

import "fmt"

type Info struct {
	Provider       string `json:"provider"`
	ID             string `json:"id"`
	IPAddress      string `json:"ip_address"`
	FreighterdPort uint16 `json:"freighterd_port"`
}

type Host interface {
	Create() (*Info, error) // create a new host
	Destroy() error         // destroy the current host
	Stop() error            // stop the current host
	Start() error           // start the current host
	Restart() error         // reboot the current host
	Info() (*Info, error)   // get current info of the host

	// get all available regions and sizes identifier for that region.
	// For example, with DigitalOcean provider:
	// {
	//	"sgp1": ["512mb", "1024mb"],
	//	"nyc2": ["1024mb", "2048mb"],
	// }
	// It's the API's caller responsibility to make sense of the data ( by
	// mapping these identifier with their predefined properties )
	Availability() (map[string][]string, error)
}

type NewHost func(defaultConfig map[string]string, config string) (Host, error)

var (
	Hosts = map[string]NewHost{}
)

// accept:
// - provider: amazon, digitalocean, etc...
// - defaultConfig: key-value strings. This is typically where we set the
// default value that we use for all boxhosts i.e access/secret key etc...
// - config: json string of user customization
func InitHost(provider string, defaultConfig map[string]string, config string) (Host, error) {
	init, ok := Hosts[provider]
	if !ok {
		return nil, fmt.Errorf("Invalid provider %v", provider)
	}
	return init(defaultConfig, config)
}
