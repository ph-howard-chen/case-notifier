# USCIS Case Tracker Implementation Plan

## Project Overview

Build a Go service that monitors USCIS case status and sends email notifications when changes are detected.

**Requirements from Notion:**
- Poll `https://my.uscis.gov/account/case-service/api/cases/<case_id>` every 5 minutes
- Detect changes in response and send email notifications via Resend API
- Handle authentication errors (401) gracefully
- Use Bazel for build management
- Deploy to GCP Cloud Run
- Make open source for community use

---

## Phase 1: Project Setup & Core Structure

### 1. Initialize Go Module
- Create `go.mod` with module name `github.com/phhowardchen/case-tracker`
- Initialize with Go 1.21+

### 2. Set up Bazel Workspace
- Create `WORKSPACE` file with `rules_go` and `gazelle`
- Configure `BUILD.bazel` files for Go targets
- Set up `gazelle` for automatic BUILD file generation

### 3. Create Directory Structure
```
case-tracker/
├── cmd/
│   └── tracker/          # Main application entry point
│       ├── main.go
│       └── BUILD.bazel
├── internal/
│   ├── uscis/           # USCIS API client
│   │   ├── client.go
│   │   ├── client_test.go
│   │   └── BUILD.bazel
│   ├── notifier/        # Resend email client
│   │   ├── resend.go
│   │   ├── resend_test.go
│   │   └── BUILD.bazel
│   ├── storage/         # State persistence
│   │   ├── storage.go
│   │   ├── storage_test.go
│   │   └── BUILD.bazel
│   └── config/          # Configuration management
│       ├── config.go
│       ├── config_test.go
│       └── BUILD.bazel
├── WORKSPACE
├── BUILD.bazel
├── go.mod
├── go.sum
└── README.md
```

---

## Phase 2: Core Service Implementation (Milestone 1)

### 4. Configuration Management
**File:** `internal/config/config.go`
- Load configuration from environment variables:
  - `USCIS_COOKIE` - Authentication cookie for USCIS API
  - `CASE_ID` - USCIS case ID to monitor
  - `RESEND_API_KEY` - Resend API authentication key
  - `RECIPIENT_EMAIL` - Email address for notifications (default: gtoshiba011@gmail.com)
  - `POLL_INTERVAL` - Optional, defaults to 5 minutes
  - `STATE_FILE_PATH` - Optional, defaults to `/tmp/case-tracker-state.json`
- Validation for required fields

### 5. USCIS API Client
**File:** `internal/uscis/client.go`
- `Client` struct with HTTP client
- `FetchCaseStatus(caseID, cookie string)` method
- Returns JSON response as map or struct
- Handle HTTP errors, network issues
- Add custom User-Agent header
- Include cookie in request headers

### 6. Resend Email Client
**File:** `internal/notifier/resend.go`
- `ResendClient` struct
- `SendEmail(to, subject, body string)` method
- Integration with Resend API v1
- Support HTML and plain text email bodies
- Error handling and logging

### 7. Main Service - Scheduler
**File:** `cmd/tracker/main.go`
- Load configuration
- Initialize USCIS client, Resend client, storage
- Create ticker for 5-minute intervals
- On each tick:
  - Fetch current case status
  - Format response as email
  - Send email via Resend
- Graceful shutdown on SIGTERM/SIGINT

### 8. Bazel BUILD Files
- Generate BUILD files using `gazelle`
- Binary target: `//cmd/tracker:tracker`
- Library targets for each internal package
- Test targets: `*_test` rules

---

## Phase 3: Change Detection & Error Handling (Milestone 2)

### 9. State Storage
**File:** `internal/storage/storage.go`
- `Storage` interface with `Load()` and `Save()` methods
- `FileStorage` implementation using JSON
- Store last known case status
- Handle file not found (first run)
- Atomic writes to prevent corruption

### 10. Change Detector
**File:** `internal/uscis/client.go` or separate `detector.go`
- `DetectChanges(previous, current map[string]interface{})` function
- Deep comparison of JSON structures
- Return list of changed fields with old/new values
- Ignore timestamp/metadata fields that always change

### 11. Smart Notifications
**Update:** `cmd/tracker/main.go`
- Load previous state from storage
- Only send email if changes detected
- Email should include:
  - Subject: "USCIS Case Status Update - [Case ID]"
  - Body: List of what changed (before → after)
  - Full JSON response for reference
- Save new state after successful notification

### 12. 401 Error Handler
**Update:** `internal/uscis/client.go`
- Detect HTTP 401 Unauthorized response
- Return specific error type: `ErrAuthenticationFailed`
- In main loop: catch this error
- Send alert email: "Cookie Expired - Action Required"
- Log error and exit gracefully (non-zero exit code)

### 13. Unit Tests
**Files:** `*_test.go` throughout
- Test config loading and validation
- Mock HTTP responses for USCIS client
- Test change detection with various scenarios
- Test email formatting
- Test state persistence
- Follow guideline: re-run tests after any test file changes using `go test ./package/ -run TestSpecificFunction`

---

## Phase 4: Function Enhancement

### 14. Multi-Case Configuration
**Update:** `internal/config/config.go`
- Change `CaseID string` to `CaseIDs []string`
- Change `StateFilePath string` to `StateFileDir string`
- Parse `CASE_IDS` environment variable as comma-separated list
- Default `STATE_FILE_DIR` to `/tmp/case-tracker-states/`
- Validate at least one case ID is provided
- Example: `CASE_IDS=IOE0933798378,IOE0944567890,IOE0955123456`

### 15. Enhanced State Storage with Timestamps
**Update:** `internal/storage/storage.go`
- Add case ID parameter to `NewFileStorage(stateDir, caseID string)`
- State file naming: `{STATE_FILE_DIR}/case-{caseID}-{YYYYMMDD-HHMMSS}.json`
- Generate readable timestamp suffix for each state file
- Keep historical states instead of overwriting
- Ensure directory creation
- Load latest state by reading most recent timestamp file

### 16. Multi-Case Main Loop
**Update:** `cmd/tracker/main.go`
- Loop through each case ID from `config.CaseIDs`
- For each case:
  - Create separate `FileStorage` instance with case ID
  - Load previous state for this case (latest by timestamp)
  - Fetch and check status independently
  - Send separate email per case (clearer than grouping)
- Add case ID to all log messages: `log.Printf("[Case: %s] ...", caseID)`
- Process cases sequentially (simpler) or concurrently with goroutines (faster)
- Handle errors per case without stopping other cases

### 17. Update Configuration Examples
**Update:** `.env.example`
- Change `CASE_ID` to `CASE_IDS` (comma-separated)
- Change `STATE_FILE_PATH` to `STATE_FILE_DIR`
- Document backward compatibility break
- Example template:
```bash
CASE_IDS=IOE0933798378,IOE0944567890
STATE_FILE_DIR=/tmp/case-tracker-states/
```

---

## Phase 5: Automated Login (Basic - No 2FA)

### 18. USCIS Authentication Module (Basic Login)
**File:** `internal/uscis/auth.go`
- `Login(username, password string) (string, error)`
  - Performs login WITHOUT 2FA handling
  - Returns session cookie value
  - Uses `http.Client` with `cookiejar.New()` for automatic cookie management
- `extractCSRFToken(loginPageHTML string) (string, error)`
  - Scrape login page for CSRF token if needed
  - Return empty string if no CSRF required
- `submitLogin(username, password, csrfToken string) (*http.Response, []*http.Cookie, error)`
  - Submit login credentials to USCIS login endpoint
  - Return response and all cookies from cookie jar
  - Handle basic error cases (invalid credentials, network errors)
- `extractSessionCookie(cookies []*http.Cookie, cookieName string) (string, error)`
  - Extract specific cookie by name from cookie jar (`_myuscis_session_rx`)
  - Return full cookie format: `_myuscis_session_rx=<value>`
- Implement retry logic with exponential backoff for network errors
- **Note:** This version assumes login succeeds without 2FA challenge

### 19. Enhanced USCIS Client (Basic Auto-Login)
**Update:** `internal/uscis/client.go`
- Add `NewClientWithAutoLogin(username, password string) (*Client, error)`
  - Calls basic `auth.Login()` without 2FA
  - Returns authenticated client with session cookie
  - Store credentials for future refresh
- Add `RefreshSession() error`
  - Re-authenticate using stored credentials
  - Called automatically on 401 errors
  - Updates client's cookie with new session
- Keep `NewClient(cookie string)` for backward compatibility
- Add private fields to store auto-login credentials:
  - `username string`
  - `password string`
  - `autoLoginEnabled bool`
- Modify `FetchCaseStatus()`:
  - On 401 error, check if `autoLoginEnabled`
  - If true, call `RefreshSession()` and retry request
  - If false or refresh fails, return `ErrAuthenticationFailed`

### 20. Configuration for Basic Auto-Login
**Update:** `internal/config/config.go`
- Add new fields to `Config` struct:
  - `AutoLogin bool` - Enable/disable auto-login mode
  - `USCISUsername string` - USCIS account username
  - `USCISPassword string` - USCIS account password
- Parse new environment variables:
  - `AUTO_LOGIN` - Set to "true" to enable (default: false)
  - `USCIS_USERNAME` - USCIS account username
  - `USCIS_PASSWORD` - USCIS account password
- Validation logic:
  - If `AUTO_LOGIN=true`, require `USCIS_USERNAME` and `USCIS_PASSWORD`
  - If `AUTO_LOGIN=false` or unset, require `USCIS_COOKIE` (current behavior)
  - Fail fast with clear error messages
- **Note:** No email-related fields yet (Phase 6)

### 21. Update Main Application for Basic Auto-Login
**Update:** `cmd/tracker/main.go`
- Check `cfg.AutoLogin` flag on startup
- Initialize USCIS client based on mode:
  - Auto-login mode: Call `uscis.NewClientWithAutoLogin(username, password)`
  - Manual mode: Call `uscis.NewClient(cookie)` (existing behavior)
- Log which authentication mode is active:
  - "Using auto-login mode (username/password)"
  - "Using manual cookie mode"
- Error handling in `checkAndNotifyCase()`:
  - On 401 error, client will auto-refresh if auto-login enabled
  - If refresh succeeds, continue polling
  - If refresh fails, send cookie expired email (existing behavior)
- **Note:** No 2FA handling yet

### 22. Update Configuration Examples
**Update:** `.env.example`
- Add authentication mode section
- Document two modes: Manual Cookie vs. Basic Auto-Login
- Example:
```bash
# Authentication Mode
AUTO_LOGIN=false

# Manual Cookie Mode (AUTO_LOGIN=false)
USCIS_COOKIE='_myuscis_session_rx=...'

# Auto-Login Mode (AUTO_LOGIN=true) - Basic (No 2FA support yet)
USCIS_USERNAME=your_username
USCIS_PASSWORD=your_password

# Note: If your account requires 2FA, use manual cookie mode for now
# 2FA support will be added in Phase 6
```

### 23. Dependencies and Build
- No new dependencies needed for basic login
- Standard library packages sufficient: `net/http`, `net/http/cookiejar`, `net/url`
- Run `gazelle` to update BUILD.bazel files

### 24. Testing Basic Login Flow
**Files:** `internal/uscis/auth_test.go`
- Unit tests with mocked HTTP responses for login flow
- Test successful login and cookie extraction
- Test CSRF token extraction (if required)
- Test cookie extraction by name from cookie jar
- Test error cases (invalid credentials, network errors)
- Test retry logic with exponential backoff
- Integration test with test credentials (if available)
- Manual testing with real USCIS account (WITHOUT 2FA)

### **Phase 5 Summary: Why Auto-Login Needs a Real Browser**

**The Challenge: AWS WAF JavaScript Puzzle**

When you try to login to USCIS with simple code, AWS WAF (Web Application Firewall) responds with:

```bash
$ curl -X POST 'https://myaccount.uscis.gov/v1/authentication/sign_in' \
  -H 'Content-Type: application/json' \
  -d '{"email": "test@example.com", "password": "test"}'

HTTP/2 202
x-amzn-waf-action: challenge  ← "Solve this puzzle first!"
content-length: 0              ← No response body
```

**What does "challenge" mean?**

AWS WAF is saying: "Before I let you login, prove you're a real browser, not a bot."

Think of it like a CAPTCHA, but invisible:
1. 🧩 WAF gives you a JavaScript puzzle
2. 💻 Your browser must run the JavaScript to solve it
3. ✅ Only after solving it can you submit login credentials

**Why `curl` or `net/http` can't work:**

```
Simple HTTP client (curl/net/http):
  ❌ Can't run JavaScript
  ❌ Can't solve the puzzle
  ❌ Can't login

Real browser (Chrome):
  ✅ Runs JavaScript automatically
  ✅ Solves AWS WAF puzzle
  ✅ Can login successfully
```

**Solution: Use chromedp (Chrome automation)**

We use **chromedp** to launch a real Chrome browser programmatically:

```go
// chromedp opens Chrome, navigates to login page, fills form, extracts cookies
cookie, err := uscis.Login(username, password)
```

What happens behind the scenes:
1. Chrome opens and loads `https://myaccount.uscis.gov/sign-in`
2. AWS WAF JavaScript runs automatically (puzzle solved!)
3. chromedp fills in email/password fields
4. chromedp clicks "Sign In" button
5. chromedp extracts the session cookie from browser
6. Returns cookie to your Go program

**Browser Mode: Headless vs Non-Headless**

| Mode             | What it means                     | Works on Mac? | Works in Cloud Run? |
|------------------|-----------------------------------|---------------|---------------------|
| **Headless**     | Chrome runs invisibly             | yes           | Maybe (not tested)  |
| **Non-Headless** | Chrome opens a window you can see | Yes           | No (no display)     |

**Cloud Run Problem:**

Cloud Run containers don't have a display, so:
- Non-headless mode won't work (can't open window)
- Headless mode might work, but adds complexity:
  - Larger Docker image (+300-500MB for Chrome)
  - More memory needed (512MB-1GB)
  - Harder to debug if something breaks

**aws-waf-token:**
This proves your chromedp browser successfully solved the AWS WAF JavaScript challenge. This is often the hardest part, so this is a major success.
**bm_sv and ak_bmsc:** These are from Akamai Bot Manager, another advanced anti-bot service. Getting these cookies means you have also passed Akamai's initial browser checks.

---

## Phase 6: Add 2FA Support

**Key Discovery:** After login, USCIS redirects to `https://myaccount.uscis.gov/auth` when 2FA is required. Use URL detection (`/auth` in URL) to determine if 2FA verification is needed.

### 25. Email IMAP Client
**File:** `internal/email/imap.go`
- Create new package `internal/email/` for fetching 2FA codes from email
- `NewIMAPClient(server, username, password string) (*IMAPClient, error)`
  - Connect to IMAP server with TLS
  - Support Gmail, Outlook, Yahoo, and other standard IMAP providers
- `FetchLatest2FACode(senderEmail string, maxWaitTime time.Duration) (string, error)`
  - Search INBOX for recent emails from specific sender (last 5 minutes)
  - Poll every 10 seconds until email arrives or timeout
  - Parse email body and extract 6-digit code using regex
  - Return code or timeout error
- `Close() error` - Disconnect from IMAP server
- **Dependencies:** Add `github.com/emersion/go-imap/v2` to go.mod

### 26. Update USCIS Authentication for 2FA Detection
**Update:** `internal/uscis/auth.go`
- Add constant: `twoFAPageURL = "https://myaccount.uscis.gov/auth"`
- Update `Login()` signature to accept optional `imapClient` parameter
- **2FA Detection Logic:**
  1. After clicking Sign In, wait 10 seconds for AWS WAF + login processing
  2. Check current URL - if contains `/auth` → 2FA required
  3. If 2FA not required → extract cookie and return (done)
- **2FA Handling Logic:**
  1. If 2FA required but no imapClient provided → return error
  2. Fetch verification code from email via IMAP
  3. Find verification input field in browser and submit code
  4. Wait for verification to complete
  5. Extract fully authenticated cookie
- **Important:** 10-second sleep is necessary because checking for `aws-waf-token` cookie alone is insufficient - it appears early but doesn't mean login is complete

### 27. Update Configuration for 2FA
**Update:** `internal/config/config.go`
- Add email-related fields to `Config` struct: `EmailIMAPServer`, `EmailUsername`, `EmailPassword`, `Email2FASender`, `Email2FATimeout`
- Parse environment variables: `EMAIL_IMAP_SERVER`, `EMAIL_USERNAME`, `EMAIL_PASSWORD`, `EMAIL_2FA_SENDER`, `EMAIL_2FA_TIMEOUT`
- Validation: If any email field is set, require all email fields to be present
- Email settings are optional (for accounts without 2FA or manual cookie mode)

### 28. Update USCIS Client for 2FA
**Update:** `internal/uscis/client.go`
- Update `NewClientWithAutoLogin()` to accept optional `imapClient` parameter
- Store `imapClient` in Client struct for session refresh
- Pass `imapClient` to `auth.Login()`
- Update `RefreshSession()` to use stored imapClient

### 29. Update Main Application for 2FA
**Update:** `cmd/tracker/main.go`
- If `AUTO_LOGIN=true` and email settings configured:
  - Create `email.IMAPClient` with credentials
  - Pass to `NewClientWithAutoLogin()`
  - Log: "2FA support enabled"
- If email settings not configured:
  - Pass nil for imapClient
  - Log: "2FA not configured"
- Handle 2FA errors gracefully

### 30. Update Configuration Examples
**Update:** `.env.example`
- Add email configuration section for 2FA support
- Document Gmail app password setup instructions
- Mark email fields as optional
- Example fields: `EMAIL_IMAP_SERVER`, `EMAIL_USERNAME`, `EMAIL_PASSWORD`, `EMAIL_2FA_SENDER`, `EMAIL_2FA_TIMEOUT`

### 31. Dependencies and Build
- Add `github.com/emersion/go-imap/v2` to go.mod
- Run `go mod tidy` and `go mod vendor`
- Update Bazel WORKSPACE if needed

### 32. Testing 2FA Flow
- Unit tests: IMAP client, 2FA detection, code parsing
- Integration tests: End-to-end with mocked IMAP server
- Manual testing: Real USCIS account with 2FA enabled
- Verify final cookie works for case status API (no 401 errors)

---

## Phase 7: Containerization & Deployment

### 33. Dockerfile
- Multi-stage build:
  1. Build stage: Use Bazel to build binary
  2. Runtime stage: Minimal base image (distroless or alpine)
- Copy binary to runtime image
- Set entrypoint to `/tracker`
- No EXPOSE needed for Cloud Run (it's a worker, not a server)

### 34. Cloud Run Configuration
**Option A: Using Cloud Build**
- Create `cloudbuild.yaml`:
  - Build Docker image
  - Push to Google Container Registry (GCR)
  - Deploy to Cloud Run

**Option B: Manual deployment scripts**
- `deploy.sh` script with `gcloud run deploy` commands

**Cloud Run Settings:**
- CPU always allocated (not request-based)
- Minimum 1 instance (keep alive for polling)
- Memory: 256Mi (lightweight service)
- Environment variables for secrets
- Use Secret Manager for sensitive values

### 35. Environment Setup Documentation
**In README.md:**
- GCP project setup
- Enable Cloud Run API
- Create service account with necessary permissions
- Store secrets in Secret Manager
- Deploy command examples

### 36. .gitignore
Exclude:
- `bazel-*` directories
- `*.json` (state files)
- `.env` (local config)
- `*.log`
- Bazel cache directories

---

## Phase 8: Open Source Preparation (Milestone 3)

### 37. README.md
Sections:
- Project overview and purpose
- Prerequisites (Go, Bazel, GCP account, Resend account)
- How to get USCIS cookie (browser DevTools instructions)
- Local setup instructions
- Configuration (environment variables)
- Running locally: `bazel run //cmd/tracker`
- Deployment to Cloud Run (step-by-step)
- Troubleshooting section
- Contributing link

### 38. LICENSE
- Choose MIT License (most permissive for community use)
- Add copyright notice

### 39. CONTRIBUTING.md
- Code style guidelines
- How to submit issues
- Pull request process
- Testing requirements
- Branch naming convention (howardchen-* per global config)

### 40. .env.example Template
Template file:
```bash
USCIS_COOKIE=your_cookie_here
CASE_IDS=IOE0933798378
RESEND_API_KEY=re_xxxxxxxxxxxx
RECIPIENT_EMAIL=your-email@example.com
POLL_INTERVAL=5m
STATE_FILE_DIR=/tmp/case-tracker-states/
```

### 41. Architecture Documentation
**Create docs/ARCHITECTURE.md:**
- System diagram (ASCII or link to diagram)
- Component descriptions
- Data flow
- Error handling strategy
- State management approach
- Why Bazel? Why Cloud Run?

---

## Technical Decisions & Rationale

### Configuration Storage
**Decision:** Environment variables
**Rationale:**
- Standard for 12-factor apps
- Cloud Run native support
- Easy integration with Secret Manager
- No file-based secrets to manage

### State Storage
**Decision:** JSON file in persistent volume or `/tmp`
**Rationale:**
- Simple, no external database needed
- Cloud Run supports volume mounts
- Easy to inspect and debug
- For `/tmp`: acceptable to lose state on container restart (just re-sends notification)

### Scheduling Mechanism
**Decision:** Simple `time.Ticker` in Go
**Rationale:**
- No external scheduler needed (Cloud Scheduler, cron)
- Cloud Run keeps container alive with min instances
- Simpler deployment
- Alternative: Use Cloud Scheduler to trigger HTTP endpoint if preferred

### Error Handling
**Decision:** Structured logging + graceful shutdown on auth failure
**Rationale:**
- Auth failures require manual intervention (new cookie)
- Better to stop than repeatedly fail
- Cloud Run will restart with exponential backoff
- Logging to Cloud Logging for debugging

### Bazel Build System
**Decision:** Use `rules_go` v0.42.0+ and `gazelle`
**Rationale:**
- Per requirement
- Reproducible builds
- Fast incremental builds
- Easy dependency management
- Native Go toolchain integration

---

## Implementation Order

For **Milestone 1** (Basic functionality):
1. Set up project structure (Phase 1)
2. Implement config, USCIS client, Resend client (Phase 2: items 4-6)
3. Basic main loop that fetches and emails every 5 minutes (Phase 2: item 7)
4. Bazel BUILD files (Phase 2: item 8)
5. Test locally with real credentials

For **Milestone 2** (Smart notifications):
6. Add state storage (Phase 3: item 9)
7. Implement change detection (Phase 3: item 10)
8. Update main loop for smart notifications (Phase 3: item 11)
9. Add 401 error handling (Phase 3: item 12)
10. Write unit tests (Phase 3: item 13)

For **Milestone 2.5** (Function Enhancement):
11. Support multiple case IDs (Phase 4: item 14)
12. Enhanced state storage with timestamps (Phase 4: items 15-16)
13. Update configuration examples (Phase 4: item 17)

For **Milestone 2.75** (Basic Auto-Login):
14. Implement USCIS authentication module - basic login (Phase 5: item 18)
15. Update USCIS client for basic auto-login (Phase 5: item 19)
16. Update configuration for basic auto-login (Phase 5: item 20)
17. Update main application (Phase 5: item 21)
18. Update .env.example (Phase 5: item 22)
19. Update dependencies and test (Phase 5: items 23-24)

For **Milestone 2.8** (Add 2FA Support):
20. Create email IMAP client (Phase 6: item 25)
21. Update USCIS authentication for 2FA detection (Phase 6: item 26)
22. Update configuration for email/2FA (Phase 6: item 27)
23. Update USCIS client to pass IMAP client (Phase 6: item 28)
24. Update main application for 2FA (Phase 6: item 29)
25. Update .env.example with email settings (Phase 6: item 30)
26. Update dependencies and test (Phase 6: items 31-32)

For **Milestone 3** (Production & Open Source):
27. Create Dockerfile (Phase 7: item 33)
28. Set up Cloud Run deployment (Phase 7: items 34-35)
29. Write documentation (Phase 8: items 37-41)
30. Add LICENSE and contributing guidelines (Phase 8: items 38-39)
31. Create .env.example template (Phase 8: item 40)
32. Final testing and release

---

## Dependencies

### Go Packages
- `github.com/resend/resend-go` - Resend SDK
- `github.com/emersion/go-imap/v2` - IMAP client for email 2FA code retrieval
- Standard library: `net/http`, `encoding/json`, `time`, `os`, `log`

### Bazel Rules
- `rules_go` - Go compilation
- `gazelle` - BUILD file generation
- `rules_docker` (optional) - Container image building

### External Services
- Resend API (email delivery)
- USCIS API (case status)
- GCP Cloud Run (hosting)
- GCP Secret Manager (optional, for secrets)

---

## Security Considerations

1. **Never commit credentials** - Use .gitignore, document in README
2. **Cookie expiration** - Handle gracefully, notify user
3. **API rate limiting** - 5-minute interval is conservative
4. **Input validation** - Validate case ID format
5. **Secret management** - Use GCP Secret Manager for production
6. **Minimal permissions** - Cloud Run service account with least privilege
7. **Credential storage for auto-login** - Store USCIS credentials in environment variables or Secret Manager, never commit
8. **Email app passwords** - Require app-specific passwords for IMAP access, not main account passwords
9. **2FA timing** - Handle delayed email delivery gracefully with configurable timeouts
10. **Login rate limiting** - Implement exponential backoff on failed login attempts to avoid account lockout

---

## Testing Strategy

1. **Unit tests** for business logic (change detection, config parsing)
2. **Integration tests** with mocked HTTP responses
3. **Manual testing** with real USCIS API (requires valid cookie)
4. **Smoke test** in Cloud Run after deployment
5. **Follow guideline**: Always re-run tests after modifying test files using targeted test execution

---

## Future Enhancements (Post-Milestone 3)

- Web UI for configuration (no need to redeploy)
- Database backend for state (PostgreSQL/Firestore)
- SMS notifications via Twilio
- Prometheus metrics export
- Slack/Discord notification options
- State file cleanup/retention policy (e.g., keep last 30 days)
- Headless browser automation as fallback (Playwright/Puppeteer) if USCIS changes API
