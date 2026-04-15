package cloud

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is an authenticated HTTP client for the VibeServe platform API.
// The caller is responsible for closing resp.Body on successful responses.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) do(method, path string, body any) (*http.Response, error) {
	var buf io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		buf = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.baseURL+path, buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, b)
	}
	return resp, nil
}

// Get issues an authenticated GET request. Caller must close resp.Body on success.
func (c *Client) Get(path string) (*http.Response, error) {
	return c.do("GET", path, nil)
}

// Post issues an authenticated POST request with a JSON body. Caller must close resp.Body on success.
func (c *Client) Post(path string, body any) (*http.Response, error) {
	return c.do("POST", path, body)
}

// Patch issues an authenticated PATCH request with a JSON body. Caller must close resp.Body on success.
func (c *Client) Patch(path string, body any) (*http.Response, error) {
	return c.do("PATCH", path, body)
}

// Delete issues an authenticated DELETE request. Caller must close resp.Body on success.
func (c *Client) Delete(path string) (*http.Response, error) {
	return c.do("DELETE", path, nil)
}
