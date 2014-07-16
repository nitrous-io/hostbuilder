package dotestv2

import (
	"net/http"

	"github.com/gorilla/mux"
)

var (
	ErrorResponse    = &response{501, `{"error":"Denied","description":"Access Denied"}`}
	NotfoundResponse = &response{404, `{"error":"NotFound","description":"Not Found"}`}
	OkResponse       = &response{200, `{"status":"OK"}`}

	urlMapping = map[string]map[string]string{
		"GET": {
			"/regions": "regions",
		},
		"PUT":    {},
		"POST":   {},
		"DELETE": {},
	}
)

// return the json string of the response
type responder func(w http.ResponseWriter, r *http.Request) *response

type response struct {
	code int
	data string
}

type Server struct {
	Router *mux.Router
	// collection of response and request per methods
	Responses map[string]map[string][]*response
	Requests  map[string]map[string][]*http.Request
	rCounter  map[string]map[string]int
}

func (s *Server) makeResponse(method, endpoint string) func(w http.ResponseWriter, r *http.Request) {
	cachedEndpoint := endpoint
	return func(w http.ResponseWriter, r *http.Request) {
		s.addRequests(method, cachedEndpoint, r)
		jsonData := s.nextResponse(method, cachedEndpoint)
		if jsonData.code == 0 {
			jsonData.code = 200
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(jsonData.code)
		w.Write([]byte(jsonData.data))
	}
}

func (s *Server) nextResponse(method, endpoint string) *response {
	responses, ok := s.Responses[method][endpoint]
	if !ok {
		return NotfoundResponse
	}
	c, ok := s.rCounter[method][endpoint]
	if !ok {
		s.rCounter[method][endpoint] = 0
	}
	if len(responses) <= int(c) {
		return NotfoundResponse
	}
	r := responses[c]
	s.rCounter[method][endpoint] += 1
	return r
}

func (s *Server) addRequests(method, endpoint string, r *http.Request) {
	_, ok := s.Requests[method][endpoint]
	if ok {
		s.Requests[method][endpoint] = append(s.Requests[method][endpoint], r)
	} else {
		s.Requests[method][endpoint] = []*http.Request{r}
	}
}

func (s *Server) AddResponse(method, endpoint string, r *response) {
	_, ok := s.Responses[method][endpoint]
	if ok {
		s.Responses[method][endpoint] = append(s.Responses[method][endpoint], r)
	} else {
		s.Responses[method][endpoint] = []*response{r}
	}
	_, ok = s.rCounter[method][endpoint]
	if !ok {
		s.rCounter[method][endpoint] = 0
	}
}

func NewServer() *Server {
	s := &Server{
		Responses: map[string]map[string][]*response{
			"GET":    {},
			"POST":   {},
			"PUT":    {},
			"DELETE": {},
		},
		Requests: map[string]map[string][]*http.Request{
			"GET":    {},
			"POST":   {},
			"PUT":    {},
			"DELETE": {},
		},
		rCounter: map[string]map[string]int{
			"GET":    {},
			"POST":   {},
			"PUT":    {},
			"DELETE": {},
		},
	}
	r := mux.NewRouter()
	for method, mapping := range urlMapping {
		for route, name := range mapping {
			r.HandleFunc(route, s.makeResponse(method, name)).Methods(method)
		}
	}
	s.Router = r
	return s

}

func NewResponse(code int, message string) *response {
	return &response{code, message}
}
