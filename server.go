package hostbuilder

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/hostbuilder/host"
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
}

type Server struct {
	Config *config
	Router *mux.Router
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

	var info *host.Info
	info, err = h.Create()
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
	err = h.Stop()
	if err != nil {
		return err
	}
	writeResponse(w, okResponse)
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
	err = h.Start()
	if err != nil {
		return err
	}
	writeResponse(w, okResponse)
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
	err = h.Restart()
	if err != nil {
		return err
	}
	writeResponse(w, okResponse)

	return nil
}

// get availability of each providers
type providerAvailablity struct {
	provider     string
	availability map[string][]string
	err          error
}

func (s *Server) availability(w http.ResponseWriter, r *http.Request) error {
	// check availability for all configured providers that we have
	avail := make(chan *providerAvailablity, len(s.Config.Providers))

	// loop over all the providers and check their availability
	for provider, config := range s.Config.Providers {
		go func(provider string, config map[string]string) {
			log.Printf("Getting availability for %s", provider)
			h, err := InitHost(provider, config, "{}")
			if err != nil {
				avail <- &providerAvailablity{
					provider: provider,
					err:      err,
				}
				return
			}

			ha, err := h.Availability()
			avail <- &providerAvailablity{
				provider:     provider,
				availability: ha,
				err:          err,
			}
		}(provider, config)
	}

	resp := make(map[string]map[string][]string)
	for _, _ = range s.Config.Providers {
		ha := <-avail
		if ha.err != nil {
			log.Printf("Failed to get availability for provider %s: %v",
				ha.provider, ha.err)
		} else {
			resp[ha.provider] = ha.availability
		}
	}
	return json.NewEncoder(w).Encode(resp)
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
		"GET": {
			"/availability": s.availability,
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
