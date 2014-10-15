package hostbuilder

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/nitrous-common/flusher"
)

var (
	okResponse = &Response{StatusCode: http.StatusOK, Status: "ok"}
)

// ProgressWriter write the stream data emitted by the host builder as new line
// separated json messages
type ProgressWriter struct {
	writer io.Writer
}

func NewProgressWriter(w io.Writer) *ProgressWriter {
	return &ProgressWriter{w}
}

func (w *ProgressWriter) Write(p []byte) (int, error) {
	err := w.WriteResponse(&Response{Stream: string(p)})
	return len(p), err
}

func (w *ProgressWriter) WriteError(err error) error {
	return w.WriteResponse(&Response{Error: err.Error()})
}

func (w *ProgressWriter) WriteResponse(r *Response) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w.writer, "%s\n", string(b))
	return err
}

// API response of the server
type Response struct {
	StatusCode     int    `json:"-"`
	Status         string `json:"status,omitempty"`
	Provider       string `json:"provider,omitempty"`
	ID             string `json:"id,omitempty"`
	IPAddress      string `json:"ip_address,omitempty"`
	FreighterdPort uint16 `json:"freighterd_port,omitempty"`
	Stream         string `json:"stream,omitempty"`
	Error          string `json:"error,omitempty"`
}

type HttpHandler func(w http.ResponseWriter, r *http.Request) error

func makeHttpHandler(handlerFunc HttpHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := handlerFunc(w, r); err != nil {
			log.Println("[ERROR] %s %s: %s", r.Method, r.URL, err)
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

// createHost streams progress data to the output. For example
//
// In success case:
//
// { "stream": "Creating key..." }
// { "stream": "Creating key...Done!" }
// { "stream": "Creating host..." }
// { "stream": "Creating host...Done!" }
// { "host": {"status": "created", "provider": "digitalocean" } }
//
// In error case:
// { "stream": "Creating key..." }
// { "stream": "Creating key...Done!" }
// { "stream": "Creating host..." }
// { "stream": "Creating host...Failed!" }
// { "error": "failed to create the host" }
func (s *Server) createHost(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	provider := r.Form.Get("provider")
	providerConfig := s.Config.Providers[provider]
	configJson := r.Form.Get("config")
	log.Printf("Creating a host using provider %s", provider)

	h, err := InitHost(provider, providerConfig, configJson)
	if err != nil {
		return err
	}

	// write the progress into the stdout so that it will be logged
	pw := NewProgressWriter(io.MultiWriter(os.Stdout, flusher.NewWriteFlusher(w)))

	info, err := h.Create(pw)
	log.Printf("Created: %+v\n, %v", info, err)
	if err != nil {
		pw.WriteError(fmt.Errorf("failed to create a host on %s", provider))
		return nil
	}
	resp := &Response{
		StatusCode:     http.StatusCreated,
		Status:         "created",
		Provider:       info.Provider,
		ID:             info.ID,
		IPAddress:      info.IPAddress,
		FreighterdPort: info.FreighterdPort,
	}
	pw.WriteResponse(resp)
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
