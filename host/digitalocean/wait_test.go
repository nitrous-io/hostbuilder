package digitalocean_test

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/digitalocean/godo"
	"github.com/nitrous-io/hostbuilder/host/digitalocean"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

var _ = Describe("wait helpers", func() {
	Context("WaitForAction", func() {
		It("waits until the action is completed", func() {
			var service godo.ActionsService = testActionService{
				1: {
					responses: []*actionResponse{
						&actionResponse{&godo.Action{Status: "in-progress"}, nil},
						&actionResponse{&godo.Action{Status: "completed"}, nil},
					},
				},
			}

			a, err := digitalocean.WaitForAction(service, 1, time.Millisecond, 5*time.Millisecond)
			Expect(err).To(BeNil(), "unexpected error")
			Expect(a.Status).To(Equal("completed"))
		})

		It("returns error when we cant retrieve the action", func() {
			var service godo.ActionsService = testActionService{
				1: {
					responses: []*actionResponse{
						&actionResponse{&godo.Action{Status: "in-progress"}, nil},
						&actionResponse{nil, errors.New("invalid response")},
					},
				},
			}

			_, err := digitalocean.WaitForAction(service, 1, time.Millisecond, 5*time.Millisecond)
			Expect(err).ToNot(BeNil(), "expect error when failed to get the action")
		})

		It("returns error when the action failed to complete", func() {
			var service godo.ActionsService = testActionService{
				1: {
					responses: []*actionResponse{
						&actionResponse{&godo.Action{Status: "in-progress"}, nil},
						&actionResponse{&godo.Action{Status: "errored"}, nil},
					},
				},
			}

			_, err := digitalocean.WaitForAction(service, 1, time.Millisecond, 5*time.Millisecond)
			Expect(err).ToNot(BeNil(), "expect error when action status is errored")
		})

		It("times out when the action takes too long to complete", func() {
			var service godo.ActionsService = testActionService{
				1: {
					responses: []*actionResponse{
						&actionResponse{&godo.Action{Status: "in-progress"}, nil},
						&actionResponse{&godo.Action{Status: "in-progress"}, nil},
						&actionResponse{&godo.Action{Status: "in-progress"}, nil},
					},
				},
			}

			_, err := digitalocean.WaitForAction(service, 1, time.Millisecond, 2*time.Millisecond)
			Expect(err).ToNot(BeNil(), "expect error timed out")
		})
	})

	Context("WaitForServer", func() {
		It("should immediately return true if no attempts are required", func(done Done) {
			Expect(digitalocean.WaitForServer("", 100*time.Millisecond, []time.Duration{})).To(BeTrue())
			close(done)
		}, 0.2)

		It("should return true if the server is up and returns 200", func() {
			server := ghttp.NewTLSServer()
			defer server.Close()
			server.AppendHandlers(
				ghttp.VerifyRequest("GET", "/"),
			)

			attemptTimes := []time.Duration{50 * time.Millisecond, 50 * time.Millisecond}

			Expect(digitalocean.WaitForServer(server.URL(), 100*time.Millisecond, attemptTimes)).To(BeTrue())
			Expect(server.ReceivedRequests()).To(HaveLen(1))
		})

		It("should return false if the server times out", func(done Done) {
			clientTimeout := 100

			timeoutHandler := ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/"),
				func(w http.ResponseWriter, req *http.Request) {
					time.Sleep(time.Duration(clientTimeout+1) * time.Millisecond)
				},
			)

			server := ghttp.NewTLSServer()
			defer server.Close()
			server.AppendHandlers(timeoutHandler, timeoutHandler)

			attemptTimes := []time.Duration{50 * time.Millisecond, 50 * time.Millisecond}

			start := time.Now()
			Expect(digitalocean.WaitForServer(server.URL(), time.Duration(clientTimeout)*time.Millisecond, attemptTimes)).To(BeFalse())
			Expect(time.Now()).To(BeTemporally(">=", start.Add(time.Duration(2*clientTimeout+2*50)*time.Millisecond)))
			Expect(server.ReceivedRequests()).To(HaveLen(2))
			close(done)
		})

		It("should return false if the server is up but does not return 200", func() {
			unavailableHandler := ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/"),
				ghttp.RespondWith(http.StatusServiceUnavailable, ""),
			)

			server := ghttp.NewTLSServer()
			defer server.Close()
			server.AppendHandlers(unavailableHandler, unavailableHandler)

			attemptTimes := []time.Duration{50 * time.Millisecond, 50 * time.Millisecond}

			Expect(digitalocean.WaitForServer(server.URL(), 100*time.Millisecond, attemptTimes)).To(BeFalse())
			Expect(server.ReceivedRequests()).To(HaveLen(2))
		})
	})
})

type actionResponses struct {
	counter   int
	responses []*actionResponse
}

type actionResponse struct {
	action *godo.Action
	err    error
}

type testActionService map[int]*actionResponses

func (t testActionService) List(opts *godo.ListOptions) ([]godo.Action, *godo.Response, error) {
	return nil, nil, nil
}

func (t testActionService) Get(id int) (*godo.Action, *godo.Response, error) {
	a, ok := t[id]
	if !ok {
		return nil, nil, fmt.Errorf("failed to find %d", id)
	}
	a.counter = a.counter + 1
	if a.counter > len(a.responses) {
		return nil, nil, fmt.Errorf("requested %d number of actions but only got %d", a.counter, len(a.responses))
	}
	r := a.responses[a.counter-1]
	return r.action, nil, r.err
}
