package hostbuilder

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

var (
	okResponse = &Response{StatusCode: http.StatusOK, Status: "ok"}
)

// API response of the server
type Response struct {
	StatusCode     int    `json:"-"`
	Status         string `json:"status"`
	Provider       string `json:"provider,omitempty"`
	ID             string `json:"id,omitempty"`
	IPAddress      string `json:"ip_address,omitempty"`
	FreighterdPort uint16 `json:"freighterd_port,omitempty"`
}

type HttpHandler func(w http.ResponseWriter, r *http.Request) error

func makeHttpHandler(handlerFunc HttpHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := handlerFunc(w, r); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	}
}

type providerConfig map[string]string

type config struct {
	Address   string                    `json:"address"`
	Providers map[string]providerConfig `json:"providers"`
	SSLPaths  map[string]string         `json:"ssl"`
}

type Server struct {
	Config *config
	Router *mux.Router
}

type CertPaths struct {
	CA   string
	Cert string
	Key  string
}

func (s *Server) sslPaths() *CertPaths {
	return &CertPaths{
		CA:   s.Config.SSLPaths["cafile"],
		Cert: s.Config.SSLPaths["certfile"],
		Key:  s.Config.SSLPaths["keyfile"],
	}
}

func (s *Server) TLSConfig() *tls.Config {
	cp := s.sslPaths()
	cert, err := tls.LoadX509KeyPair(cp.Cert, cp.Key)
	if err != nil {
		log.Printf("hostbuilder: Failed to load X509 keypair - %s & %s: %s", cp.Cert, cp.Key, err)
	}
	tlsConfig := &tls.Config{
		NextProtos:   []string{"http/1.1"},
		Certificates: []tls.Certificate{cert},
	}

	certPool := x509.NewCertPool()
	caFile, err := ioutil.ReadFile(cp.CA)
	if err != nil {
		log.Printf("hostbuilder: Failed to read CA certificate, %s: %s", cp.CA, err)
	}

	certPool.AppendCertsFromPEM(caFile)

	tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	tlsConfig.ClientCAs = certPool
	return tlsConfig
}

func (s *Server) createHost(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	provider := r.Form.Get("provider")
	providerConfig := s.Config.Providers[provider]
	configJson := r.Form.Get("config")

	h, err := InitHost(provider, providerConfig, configJson)
	if err != nil {
		return err
	}

	info, err := h.Create()
	log.Printf("Created: %+v\n, %v", info, err)
	if err != nil {
		return err
	}
	resp := &Response{
		StatusCode:     http.StatusCreated,
		Status:         "created",
		Provider:       info.Provider,
		ID:             info.ID,
		IPAddress:      info.IPAddress,
		FreighterdPort: info.FreighterdPort,
	}
	writeResponse(w, resp)
	return nil
}

func (s *Server) deleteHost(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	provider := r.Form.Get("provider")
	providerConfig := s.Config.Providers[provider]
	configJson := r.Form.Get("config")

	h, err := InitHost(provider, providerConfig, configJson)
	if err != nil {
		return err
	}
	err = h.Destroy()
	if err != nil {
		return err
	}
	writeResponse(w, okResponse)
	return nil
}

func (s *Server) stopHost(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	provider := r.Form.Get("provider")
	providerConfig := s.Config.Providers[provider]
	configJson := r.Form.Get("config")

	h, err := InitHost(provider, providerConfig, configJson)
	if err != nil {
		return err
	}
	info, err := h.Stop()
	if err != nil {
		return err
	}
	resp := &Response{
		StatusCode:     http.StatusOK,
		Status:         "stopped",
		Provider:       info.Provider,
		ID:             info.ID,
		IPAddress:      info.IPAddress,
		FreighterdPort: info.FreighterdPort,
	}
	writeResponse(w, resp)
	return nil
}

func (s *Server) startHost(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	provider := r.Form.Get("provider")
	providerConfig := s.Config.Providers[provider]
	configJson := r.Form.Get("config")

	h, err := InitHost(provider, providerConfig, configJson)
	if err != nil {
		return err
	}
	info, err := h.Start()
	if err != nil {
		return err
	}
	resp := &Response{
		StatusCode:     http.StatusOK,
		Status:         "started",
		Provider:       info.Provider,
		ID:             info.ID,
		IPAddress:      info.IPAddress,
		FreighterdPort: info.FreighterdPort,
	}
	writeResponse(w, resp)
	return nil
}

func (s *Server) restartHost(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	provider := r.Form.Get("provider")
	providerConfig := s.Config.Providers[provider]
	configJson := r.Form.Get("config")

	h, err := InitHost(provider, providerConfig, configJson)
	if err != nil {
		return err
	}
	info, err := h.Restart()
	if err != nil {
		return err
	}
	resp := &Response{
		StatusCode:     http.StatusOK,
		Status:         "restarted",
		Provider:       info.Provider,
		ID:             info.ID,
		IPAddress:      info.IPAddress,
		FreighterdPort: info.FreighterdPort,
	}
	writeResponse(w, resp)
	return nil
}

func (s *Server) CreateRouter() {
	r := mux.NewRouter()
	m := map[string]map[string]HttpHandler{
		"POST": {
			"/hosts":         s.createHost,
			"/hosts/delete":  s.deleteHost,
			"/hosts/stop":    s.stopHost,
			"/hosts/start":   s.startHost,
			"/hosts/restart": s.restartHost,
		},
	}
	for method, routes := range m {
		for route, handler := range routes {
			localRoute := route
			localHandler := handler
			localMethod := method
			f := makeHttpHandler(localHandler)
			r.HandleFunc(localRoute, f).Methods(localMethod)
		}
	}
	s.Router = r
}

func New(configData []byte) (*Server, error) {
	config := &config{}
	if err := json.Unmarshal(configData, config); err != nil {
		log.Printf("hostbuilder: invalid config data %v", string(configData))
		return nil, err
	}
	server := &Server{Config: config}
	server.CreateRouter()
	return server, nil
}

func writeResponse(w http.ResponseWriter, resp *Response) {
	w.Header().Set("Content-Type", "application/json")
	data, err := json.Marshal(resp)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		data = []byte(`{"status":"error"}`)
	} else {
		w.WriteHeader(resp.StatusCode)
	}
	w.Write(data)
}
