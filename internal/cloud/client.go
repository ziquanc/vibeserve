package cloud

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

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
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, b)
	}
	return resp, nil
}

func (c *Client) Get(path string) (*http.Response, error) {
	return c.do("GET", path, nil)
}

func (c *Client) Post(path string, body any) (*http.Response, error) {
	return c.do("POST", path, body)
}

func (c *Client) Patch(path string, body any) (*http.Response, error) {
	return c.do("PATCH", path, body)
}

func (c *Client) Delete(path string) (*http.Response, error) {
	return c.do("DELETE", path, nil)
}
