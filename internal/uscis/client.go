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

// Client is the USCIS API client
type Client struct {
	httpClient       *http.Client
	cookie           string
	autoLoginEnabled bool
	username         string
	password         string
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
		httpClient:       &http.Client{},
		cookie:           cookie,
		autoLoginEnabled: false,
	}
}

// NewClientWithAutoLogin creates a new USCIS client with auto-login support
// Performs initial login and stores credentials for automatic session refresh
func NewClientWithAutoLogin(username, password string) (*Client, error) {
	// Perform initial login
	cookie, err := Login(username, password)
	if err != nil {
		return nil, fmt.Errorf("initial login failed: %w", err)
	}

	return &Client{
		httpClient:       &http.Client{},
		cookie:           cookie,
		autoLoginEnabled: true,
		username:         username,
		password:         password,
	}, nil
}

// RefreshSession re-authenticates and updates the session cookie
func (c *Client) RefreshSession() error {
	if !c.autoLoginEnabled {
		return fmt.Errorf("auto-login not enabled for this client")
	}

	cookie, err := Login(c.username, c.password)
	if err != nil {
		return fmt.Errorf("session refresh failed: %w", err)
	}

	c.cookie = cookie
	return nil
}

// FetchCaseStatus fetches the current status of a case
// Automatically retries with session refresh on 401 errors when auto-login is enabled
func (c *Client) FetchCaseStatus(caseID string) (map[string]interface{}, error) {
	result, err := c.fetchCaseStatusInternal(caseID)

	// If we get a 401 and auto-login is enabled, try to refresh and retry once
	if err != nil {
		if authErr, ok := err.(*ErrAuthenticationFailed); ok && c.autoLoginEnabled {
			fmt.Printf("Authentication failed (status %d), attempting session refresh...\n", authErr.StatusCode)

			// Try to refresh the session
			if refreshErr := c.RefreshSession(); refreshErr != nil {
				return nil, fmt.Errorf("session refresh failed: %w (original error: %v)", refreshErr, err)
			}

			fmt.Println("Session refreshed successfully, retrying request...")

			// Retry the request with new cookie
			result, err = c.fetchCaseStatusInternal(caseID)
		}
	}

	return result, err
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
