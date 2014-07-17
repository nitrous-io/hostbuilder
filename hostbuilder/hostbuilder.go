package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/nitrous-io/hostbuilder"
)

var version string

type providerConfig map[string]string

type config struct {
	Providers []map[string]providerConfig `json:"providers"`
}

func main() {
	flConfig := flag.String("config", "", "path to the config file")
	flVersion := flag.Bool("v", false, "current version")
	flag.Parse()
	if *flVersion {
		fmt.Printf("hostbuilder version %v\n", version)
		os.Exit(0)
		return
	}
	if len(*flConfig) == 0 {
		log.Printf("hostbuilder: missing config file")
		os.Exit(1)
	}
	configData, err := ioutil.ReadFile(*flConfig)
	if err != nil {
		log.Printf("hostbuilder: failed to read config file")
		os.Exit(1)
	}

	// start the server
	server, err := hostbuilder.New(configData)
	if err != nil {
		log.Printf("hostbuilder: failed to create server %v", err)
		os.Exit(1)
	}

	tlsConfig := server.TLSConfig()
	addr := server.Config.Address

	http.Handle("/", server.Router)
	log.Printf("hostbuilder: starting server at %v", addr)

	l, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf(fmt.Sprintf("hostbuilder: cannot start listener on address %s: %v", addr, err))
	}

	err = http.Serve(tls.NewListener(l, tlsConfig), nil)
	log.Fatalf("%s", err)
}
