package v2

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
)

const DIGITALOCEAN_API_URL = "https://api.digitalocean.com/v2"

// V2 error messages. This implements `error` interface
type ErrorResp struct {
	Message     string `json:"error"`
	Description string `json:"description"`
}

func (e *ErrorResp) Error() string {
	return fmt.Sprintf("Error: %s, %s", e.Message, e.Description)
}

// DigitalOcean V2 response's definitions
type (
	Region struct {
		Slug      string   `json:"slug"`
		Name      string   `json:"name"`
		Sizes     []string `json:"sizes"`
		Available bool     `json:"available"`
		Features  []string `json:"features"`
	}
)

type Client struct {
	client  *http.Client // The http client for communicating
	BaseURL string
	Token   string
}

func NewClient(token string) *Client {
	c := &Client{
		client: &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
			},
		},
		BaseURL: DIGITALOCEAN_API_URL,
		Token:   token,
	}
	return c
}

// Returns all available regions.
func (c Client) Regions() ([]*Region, error) {
	data, err := c.request("GET", "regions", url.Values{})
	if err != nil {
		return nil, err
	}
	resp := make(map[string][]*Region)
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	regions, ok := resp["regions"]
	if !ok {
		return nil, fmt.Errorf("Invalid regions response %v", resp)
	}
	return regions, nil
}

// Sends an api request and returns []byte for the response if possible
func (c Client) request(method string, endpoint string, params url.Values) ([]byte, error) {
	endpoint = fmt.Sprintf("%s/%s?%s", c.BaseURL, endpoint, params.Encode())
	req, err := http.NewRequest(method, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.Token))

	log.Printf("sending new request to digitalocean: %s", endpoint)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// if response is not in 200 range, it probably failed
	// in that base, we serialize the response into an `error`
	if resp.StatusCode >= 300 {
		errResp := &ErrorResp{}
		if err := json.Unmarshal(body, errResp); err != nil {
			err = fmt.Errorf("Invalid response %s with status code %d",
				string(body), resp.StatusCode)
			return nil, err
		}
		return nil, errResp
	}
	return body, nil
}
