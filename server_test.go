package hostbuilder_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	. "github.com/nitrous-io/hostbuilder"
	"github.com/nitrous-io/hostbuilder/host"
	"github.com/nitrous-io/hostbuilder/host/dummy"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestDigitalOceanAPI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "hostbuilder")
}

var _ = Describe("hostbuilder", func() {
	var ts *httptest.Server
	var s *Server
	var err error

	BeforeEach(func() {
		s, err = New([]byte(`{"providers":{"dummy":{}}}`))
		Expect(err).To(BeNil())
		ts = httptest.NewServer(s.Router)
	})

	AfterEach(func() {
		ts.Close()
	})

	Describe("Creating a host", func() {
		It("should successfully create a host and respond with host's info", func() {
			config, _ := json.Marshal(dummy.Host{ID: "123", IPAddress: "10.0.0.1", CreateSuccess: true})
			form := url.Values{}
			form.Set("provider", "dummy")
			form.Set("config", string(config))
			resp, err := http.PostForm(fmt.Sprintf("%s/%s", ts.URL, "hosts"), form)
			Expect(err).To(BeNil())
			Expect(resp.StatusCode).To(Equal(201))

			body, err := ioutil.ReadAll(resp.Body)
			Expect(err).To(BeNil())

			var info host.Info
			json.Unmarshal(body, &info)
			Expect(info.ID).To(Equal("123"))
			Expect(info.IPAddress).To(Equal("10.0.0.1"))
			Expect(info.FreighterdPort).To(Equal(uint16(65333)))
		})
	})

	Describe("Destroying a host", func() {
		It("should successfully destroy the host", func() {
			config, _ := json.Marshal(dummy.Host{DestroySuccess: true})
			form := url.Values{}
			form.Set("provider", "dummy")
			form.Set("config", string(config))

			resp, err := http.PostForm(fmt.Sprintf("%s/%s", ts.URL, "hosts/delete"), form)
			Expect(err).To(BeNil())
			Expect(resp.StatusCode).To(Equal(200))

			body, err := ioutil.ReadAll(resp.Body)
			Expect(err).To(BeNil())
			Expect(body).To(MatchJSON(`{"status":"ok"}`))
		})
	})

	Describe("Stopping a host", func() {
		It("should successfully stop the host", func() {
			config, _ := json.Marshal(dummy.Host{StopSuccess: true})
			form := url.Values{}
			form.Set("provider", "dummy")
			form.Set("config", string(config))
			resp, err := http.PostForm(fmt.Sprintf("%s/%s", ts.URL, "hosts/stop"), form)
			Expect(err).To(BeNil())
			Expect(resp.StatusCode).To(Equal(200))

			body, err := ioutil.ReadAll(resp.Body)
			Expect(err).To(BeNil())
			Expect(body).To(MatchJSON(`{"status":"ok"}`))
		})
	})

	Describe("Starting host", func() {
		It("should successfully start the host", func() {
			config, _ := json.Marshal(dummy.Host{StartSuccess: true})
			form := url.Values{}
			form.Set("provider", "dummy")
			form.Set("config", string(config))

			resp, err := http.PostForm(fmt.Sprintf("%s/%s", ts.URL, "hosts/start"), form)
			Expect(err).To(BeNil())
			Expect(resp.StatusCode).To(Equal(200))

			body, err := ioutil.ReadAll(resp.Body)
			Expect(err).To(BeNil())
			Expect(body).To(MatchJSON(`{"status":"ok"}`))
		})
	})

	Describe("Restarting host", func() {
		It("should successfully start the host", func() {
			config, _ := json.Marshal(dummy.Host{RestartSuccess: true})
			form := url.Values{}
			form.Set("provider", "dummy")
			form.Set("config", string(config))

			resp, err := http.PostForm(fmt.Sprintf("%s/%s", ts.URL, "hosts/restart"), form)
			Expect(err).To(BeNil())
			Expect(resp.StatusCode).To(Equal(200))

			body, err := ioutil.ReadAll(resp.Body)
			Expect(err).To(BeNil())
			Expect(body).To(MatchJSON(`{"status":"ok"}`))
		})
	})

	Describe("GET /availability", func() {
		It("gets available regions for each provider", func() {
			resp, err := http.Get(fmt.Sprintf("%s/%s", ts.URL, "/availability"))
			Expect(err).To(BeNil())
			body, _ := ioutil.ReadAll(resp.Body)
			Expect(body).To(MatchJSON(`{"dummy":{"dummy":["small", "medium"]}}`))
		})
	})
})
