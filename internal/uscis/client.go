package uscis

import (
	"net/http"
)

// Client is the USCIS API client
type Client struct {
	httpClient *http.Client
	cookie     string
}

// NewClient creates a new USCIS client
func NewClient(cookie string) *Client {
	return &Client{
		httpClient: &http.Client{},
		cookie:     cookie,
	}
}

// FetchCaseStatus fetches the current status of a case
func (c *Client) FetchCaseStatus(caseID string) (map[string]interface{}, error) {
	// TODO: implement case status fetching
	return nil, nil
}
