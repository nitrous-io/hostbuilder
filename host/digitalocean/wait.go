package digitalocean

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/digitalocean/godo"
	"github.com/nitrous-io/nitrous-common/httpclient"
)

const (
	inProgressStatus = "in-progress"
	erroredStatus    = "errored"
	completedStatus  = "completed"
)

// WaitForActions waits until the action is completed
func WaitForAction(service godo.ActionsService, id int, wait, timeout time.Duration) (*godo.Action, error) {
	// control channel to make sure that we can exit goroutine properly
	done := make(chan struct{})
	defer close(done)

	result := make(chan error, 1)
	var finalAction *godo.Action
	go func() {
		for {
			select {
			case <-done:
				// We finished, so just exit the goroutine
				return
			default:
				tmp, _, err := service.Get(id)
				if err != nil {
					result <- err
					return
				}
				log.Printf("Waiting for event %d to complete, got status %s", id, tmp.Status)
				if tmp.Status == erroredStatus {
					result <- fmt.Errorf("action %v failed", tmp)
					return
				} else if tmp.Status == completedStatus {
					finalAction = tmp
					result <- nil
					return
				}
				time.Sleep(wait)
			}
		}
	}()
	select {
	case err := <-result:
		if err != nil {
			return nil, err
		}
		return finalAction, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout while waiting to for event %d to complete", id)
	}
}

// WaitForServer takes a urlStr and tries to retrieve a 200 response from it up
// to len(attempts) times, waiting attempts[n-1] duration after attempt n.
// Note that the final element of attempts can be any value, since sleeping
// after is unnecessary.
func WaitForServer(urlStr string, clientTimeout time.Duration, attempts []time.Duration) bool {
	if len(attempts) == 0 {
		return true
	}

	client := httpclient.New("")
	tr := &http.Transport{
		DisableKeepAlives: true,
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
	}
	client.Client.Transport = tr
	client.Client.Timeout = clientTimeout
	for i, duration := range attempts {
		resp, err := client.Get(urlStr, nil)
		if err == nil && resp != nil && resp.StatusCode == http.StatusOK {
			return true
		}
		if i < len(attempts)-1 {
			time.Sleep(duration)
		}
	}
	return false
}
