package digitalocean_test

import (
	"net/http/httptest"
	"testing"

	do "github.com/nitrous-io/hostbuilder/pkg/digitalocean"
	"github.com/nitrous-io/hostbuilder/pkg/digitalocean/dotest"
	"github.com/nitrous-io/hostbuilder/pkg/digitalocean/dotestv2"
	"github.com/nitrous-io/hostbuilder/pkg/digitalocean/v2"

	. "github.com/nitrous-io/hostbuilder/host/digitalocean"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestDigitalOcean(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "host/digitalocean")
}

var (
	defaultTestConfig = map[string]string{
		"zone_id":        "zone_id",
		"base_domain":    "nitrousbox.com",
		"aws_secret_key": "secret_key",
		"aws_access_key": "access_key",
		"client_id":      "client_id",
		"api_key":        "api_key",
	}

	// /droplets/new
	newDroplet = `
	{
		"status": "OK",
		"droplet": {
			"id": 100823,
			"name": "test222",
			"image_id": 420,
			"size_id":33,
			"region_id": 1
		}
	}
	`

	// /ssh_keys/new
	newSSH = `
	{
		"status": "OK",
		"ssh_key": {
			"id": 47,
			"name": "my_key",
			"ssh_pub_key": "ssh-rsa key user@host"
		}
	}
	`

	// /regions
	allRegions = `
	{
		"status": "OK",
		"regions": [
			{ "id": 1, "name": "New York 1", "slug": "nyc1" },
			{ "id": 2, "name": "Amsterdam 1", "slug": "ams1" },
			{ "id": 3, "name": "San Francisco 1", "slug": "sfo1" }
		]
	}
	`

	// /sizes
	allSizes = `
	{
		"status": "OK",
		"sizes": [
			{ "id": 33, "name": "512MB", "slug": "512mb" },
			{ "id": 34, "name": "1GB", "slug": "1gb" },
			{ "id": 35, "name": "2GB", "slug": "2gb" },
			{ "id": 36, "name": "4GB", "slug": "4gb" },
			{ "id": 37, "name": "8GB", "slug": "8gb" },
			{ "id": 38, "name": "16GB", "slug": "16gb" }
		]
	}
	`

	// /image/id
	oneImage = `
	{
		"status": "OK",
		"image": {
			"id": 1,
			"name": "Automated Backup",
			"distribution":"Ubuntu"
		}
	}
	`
)

var _ = Describe("host/digitalocean", func() {
	var s *dotest.Server
	var ts *httptest.Server
	var c *do.Client

	BeforeEach(func() {
		s = dotest.NewServer()
		ts = httptest.NewServer(s.Router)
		c = do.NewClient("client_id", "api_key")
		c.BaseURL = ts.URL
	})

	AfterEach(func() {
		ts.Close()
	})

	Describe("Create", func() {
		BeforeEach(func() {
			s.AddResponse("ssh_keys/new", dotest.NewResponse(200, newSSH))
			s.AddResponse("sizes", dotest.NewResponse(200, allSizes))
			s.AddResponse("images/id", dotest.NewResponse(200, oneImage))
			s.AddResponse("regions", dotest.NewResponse(200, allRegions))
			s.AddResponse("droplets/new", dotest.NewResponse(200, newDroplet))
			s.AddResponse("droplets/id", dotest.NotfoundResponse)
			s.AddResponse("droplets/id/destroy", dotest.OkResponse)
			s.AddResponse("ssh_keys/id/destroy", dotest.OkResponse)
		})

		Context("Unsuccessful creation", func() {
			It("should clean up the droplet", func() {
				h := Host{Config: &HostConfig{ID: "1", ImageID: "1", RegionID: "1", SizeID: "33"}, Client: c}
				_, err := h.Create()
				Expect(err).NotTo(BeNil())

				request := s.Requests["droplets/id/destroy"][0]
				Expect(request.URL.Path).To(Equal("/droplets/100823/destroy"))

				request = s.Requests["ssh_keys/id/destroy"][0]
				Expect(request.URL.Path).To(Equal("/ssh_keys/47/destroy"))
			})
		})
	})

	Describe("Stop", func() {
		BeforeEach(func() {
			s.AddResponse("droplets/id/power_off", dotest.OkResponse)
		})

		Context("Successful Stop", func() {
			It("stops the droplet", func() {
				h := Host{Config: &HostConfig{ID: "1"}, Client: c}

				err := h.Stop()
				Expect(err).To(BeNil())

				request := s.Requests["droplets/id/power_off"][0]
				Expect(request.URL.Path).To(Equal("/droplets/1/power_off"))
			})
		})
	})

	Describe("Start", func() {
		BeforeEach(func() {
			s.AddResponse("droplets/id/power_on", dotest.OkResponse)
		})

		Context("Successful Start", func() {
			It("starts the droplet", func() {
				h := Host{Config: &HostConfig{ID: "1"}, Client: c}

				err := h.Start()
				Expect(err).To(BeNil())

				request := s.Requests["droplets/id/power_on"][0]
				Expect(request.URL.Path).To(Equal("/droplets/1/power_on"))
			})
		})
	})
})

var _ = Describe("digitalocean/v2", func() {
	var s *dotestv2.Server
	var ts *httptest.Server
	var c *v2.Client

	BeforeEach(func() {
		s = dotestv2.NewServer()
		ts = httptest.NewServer(s.Router)
		c = v2.NewClient("token")
		c.BaseURL = ts.URL
	})

	AfterEach(func() {
		ts.Close()
	})

	Describe("Availability", func() {
		It("returns the availability of regions and sizes", func() {
			regions := `{
				"regions": [
					{
						"slug": "sgp1",
						"name": "Singapore 1",
						"sizes": ["512mb", "1024mb"],
						"available": true,
						"features": ["ipv6", "virtio", "private_networking"]
					},
					{
						"slug": "nyc2",
						"name": "New York 2",
						"sizes": ["512mb"],
						"available": true,
						"features": ["ipv6", "virtio", "private_networking"]
					},
					{
						"slug": "asm1",
						"name": "Amsterdam 1",
						"sizes": ["512mb", "1024mb"],
						"available": false,
						"features": ["ipv6", "virtio", "private_networking"]
					}
				]
			}`
			s.AddResponse("GET", "regions", dotestv2.NewResponse(200, regions))

			h := Host{ClientV2: c}

			ha, err := h.Availability()
			Expect(err).To(BeNil(), "should succesfully get availability")

			Expect(ha["sgp1"]).To(Equal([]string{"512mb", "1024mb"}))
			Expect(ha["nyc2"]).To(Equal([]string{"512mb"}))
			Expect(ha).ToNot(HaveKey("asm1"))
		})
	})
})
