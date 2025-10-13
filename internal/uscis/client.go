package uscis

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	baseURL = "https://my.uscis.gov/account/case-service/api/cases"
)

// Client is the USCIS API client for manual cookie mode
type Client struct {
	httpClient *http.Client
	cookie     string
}

// ErrAuthenticationFailed is returned when the cookie has expired (401)
type ErrAuthenticationFailed struct {
	StatusCode int
}

func (e *ErrAuthenticationFailed) Error() string {
	return fmt.Sprintf("authentication failed: received status code %d (cookie may have expired)", e.StatusCode)
}

// NewClient creates a new USCIS client with manual cookie
func NewClient(cookie string) *Client {
	return &Client{
		httpClient: &http.Client{},
		cookie:     cookie,
	}
}

// FetchCaseStatus fetches the current status of a case
func (c *Client) FetchCaseStatus(caseID string) (map[string]interface{}, error) {
	return c.fetchCaseStatusInternal(caseID)
}

// fetchCaseStatusInternal performs the actual HTTP request
func (c *Client) fetchCaseStatusInternal(caseID string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/%s", baseURL, caseID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers to match browser/curl behavior
	req.Header.Set("Cookie", c.cookie)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch case status: %w", err)
	}
	defer resp.Body.Close()

	// Check for authentication errors
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, &ErrAuthenticationFailed{StatusCode: resp.StatusCode}
	}

	// Check for other HTTP errors
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	// Read and parse response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	return result, nil
}
