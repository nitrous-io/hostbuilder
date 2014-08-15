package hostbuilder

import (
	"github.com/nitrous-io/hostbuilder/host"
	_ "github.com/nitrous-io/hostbuilder/host/amazon"
	_ "github.com/nitrous-io/hostbuilder/host/digitalocean"
	_ "github.com/nitrous-io/hostbuilder/host/docker"
	_ "github.com/nitrous-io/hostbuilder/host/virtualbox"
)

func InitHost(provider string, defaultConfig map[string]string, config string) (host.Host, error) {
	return host.InitHost(provider, defaultConfig, config)
}
