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
│   │   ├── client.go           # HTTP client (manual cookie mode)
│   │   ├── browser_client.go   # chromedp client (auto-login mode)
│   │   ├── detector.go          # Change detection logic
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
├── test_login.go        # Standalone e2e test for browser login
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

## Phase 5: Automated Login with Browser Automation (COMPLETED ✅)

**Status:** ✅ Completed with chromedp browser automation and manual 2FA support

### 18. USCIS Browser Client (Replaces auth.go approach)
**File:** `internal/uscis/browser_client.go`

**Key Architectural Decision:** After discovering that extracted cookies don't work for HTTP requests (USCIS returns 401), we changed approach to **keep the browser session alive** and use it for all API calls.

- `BrowserClient` struct:
  - Keeps chromedp browser context alive
  - Stores username/password for session refresh
  - No cookie extraction - browser session is the credential

- `NewBrowserClient(username, password string) (*BrowserClient, error)`
  - Launches headless Chrome browser
  - Performs login with AWS WAF challenge handling (10-second wait)
  - Detects 2FA requirement by checking URL for `/auth`
  - Prompts user via stdin for 2FA code if needed
  - Navigates to applicant page to initialize session
  - Returns client with active browser session

- `FetchCaseStatus(caseID string) (map[string]interface{}, error)`
  - Navigates browser to API URL: `https://my.uscis.gov/account/case-service/api/cases/{caseID}`
  - Extracts JSON from `<pre>` tag
  - Auto-detects session expiration (`data: null`)
  - Automatically refreshes session and retries on auth failure

- `RefreshSession() error`
  - Re-runs entire login flow (including 2FA if needed)
  - Reuses same browser context
  - Useful for long-running polling when session expires

- `Close() error`
  - Cleans up browser resources

### 19. HTTP Client for Manual Cookie Mode
**File:** `internal/uscis/client.go`
- Simplified to only handle manual cookie mode
- Removed auto-login methods (now handled by BrowserClient)
- `NewClient(cookie string) *Client` - for manual cookie mode
- `FetchCaseStatus(caseID string)` - makes HTTP request with cookie
- Detects 401 errors and returns `ErrAuthenticationFailed`

### 20. Configuration for Dual Authentication Modes
**File:** `internal/config/config.go`
- `AutoLogin bool` - Enable/disable browser auto-login mode
- `USCISUsername string` - USCIS account username
- `USCISPassword string` - USCIS account password
- `USCISCookie string` - For manual cookie mode

Validation logic:
- If `AUTO_LOGIN=true`, require `USCIS_USERNAME` and `USCIS_PASSWORD`
- If `AUTO_LOGIN=false`, require `USCIS_COOKIE`

### 21. Main Application with Interface Abstraction
**File:** `cmd/tracker/main.go`
- `CaseStatusFetcher` interface - abstracts both client types
- Switch based on `AUTO_LOGIN` config:
  - `AUTO_LOGIN=true`: Use `BrowserClient` (chromedp)
  - `AUTO_LOGIN=false`: Use `Client` (HTTP with manual cookie)
- Properly handles browser lifecycle with `defer browserClient.Close()`
- Removed dead code for `ErrAuthenticationFailed` checks (clients handle internally)

### 22. Configuration Examples
**File:** `.env.example`
```bash
# Authentication Mode
AUTO_LOGIN=false

# Manual Cookie Mode (AUTO_LOGIN=false)
USCIS_COOKIE='_myuscis_session_rx=...'

# Browser Auto-Login Mode (AUTO_LOGIN=true)
# Supports manual 2FA via stdin prompt
USCIS_USERNAME=your_username
USCIS_PASSWORD=your_password
```

### 23. Dependencies and Build
- **chromedp**: `github.com/chromedp/chromedp` v0.14.2
- **cdproto**: `github.com/chromedp/cdproto` (for network cookie access)
- Go version: 1.23.0+
- Run `gazelle` to update BUILD.bazel files

### 24. Testing
**File:** `test_login.go` (standalone e2e test)
- Tests complete flow: login → 2FA → applicant page → API access
- Verifies browser session works for API calls

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

## Phase 6: 2FA Support (COMPLETED ✅ with Manual stdin approach)

**Status:** ✅ 2FA is already implemented in Phase 5 using manual stdin prompts

**Implemented Approach:** Manual 2FA via stdin (simpler, more reliable)
- Browser detects 2FA page by URL check (`/auth` in URL)
- Prompts user via stdin: "Enter 2FA verification code:"
- User manually enters code from email
- Browser submits code and continues
- Works reliably without email automation complexity

**Alternative Approach (Not Implemented):** Automated IMAP Email Fetching
This approach was considered but NOT implemented because:
- Adds significant complexity (IMAP client, email parsing, timeout handling)
- Requires email app passwords and IMAP configuration
- Manual stdin prompt is simpler and works well for local development
- For production, use manual cookie mode instead

### Future Enhancement: Automated Email 2FA (Optional)
If automatic 2FA code retrieval is needed in the future:

**File:** `internal/email/imap.go` (to be created)
- `NewIMAPClient(server, username, password string) (*IMAPClient, error)`
- `FetchLatest2FACode(senderEmail string, maxWaitTime time.Duration) (string, error)`
  - Search INBOX for recent emails
  - Parse and extract 6-digit verification code
  - Poll with timeout
- **Dependencies:** `github.com/emersion/go-imap/v2`

**Configuration:** `.env.example`
```bash
# Optional: Automated 2FA via email (not implemented)
# EMAIL_IMAP_SERVER=imap.gmail.com:993
# EMAIL_USERNAME=your_email@gmail.com
# EMAIL_PASSWORD=your_app_password
# EMAIL_2FA_SENDER=noreply@uscis.gov
```

**Rationale for Not Implementing:**
- Manual stdin works well for local development
- Production deployments should use manual cookie mode
- IMAP adds complexity without significant benefit
- Email delivery delays can cause timeout issues

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

## Key Architectural Discovery: Browser Session vs Cookies

**Problem Discovered During Phase 5:**
After implementing browser-based login, we discovered that **extracted cookies don't work for HTTP requests outside the browser session**.

**What We Tried:**
1. ✅ Browser login with chromedp → Success
2. ✅ Extract cookies from browser → Got `_uscis_user_session` cookie
3. ❌ Use cookie in HTTP client → **401 Unauthorized**

**Why Cookies Don't Work:**
USCIS returns HTTP 401 with:
```json
{"data":null,"error":{"userMessage":null,"developerMessage":null,"failureCode":null,"details":null,"requestId":"..."}}
```

The browser session has additional state beyond just cookies (AWS WAF tokens, Akamai fingerprints, etc.) that can't be extracted and reused.

**Solution: Keep Browser Session Alive**
Instead of extracting cookies, we:
1. Launch browser with `NewBrowserClient()`
2. Perform login (browser session becomes the "credential")
3. Keep chromedp context alive
4. Navigate browser to API URLs for each request
5. Extract JSON from `<pre>` tag in browser

**Trade-offs:**

| Approach | Pros | Cons |
|----------|------|------|
| **BrowserClient (auto-login)** | ✅ Handles AWS WAF/Akamai<br>✅ Auto-refresh on expiration<br>✅ 2FA support | ❌ Higher memory (~200-500MB)<br>❌ Slower than HTTP<br>❌ Harder for Cloud Run |
| **Client (manual cookie)** | ✅ Lightweight (~20MB)<br>✅ Fast HTTP requests<br>✅ Easy for Cloud Run | ❌ Manual cookie refresh<br>❌ No 2FA support<br>❌ Cookie expires periodically |

**Recommendation:** Use **manual cookie mode** for production (Cloud Run), **browser mode** for local development.

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
**Decision:** Client-internal auth failure handling + structured logging
**Rationale:**
- **BrowserClient**: Auto-refreshes session on 401 (re-login with 2FA if needed)
- **Client**: Returns error on 401, user must update cookie
- Main loop logs errors but continues polling other cases
- Removed dead code for `ErrAuthenticationFailed` checks in main.go
- Each client handles its own authentication strategy
- Logging to stdout for debugging (Cloud Logging in production)

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

### ✅ COMPLETED

**Milestone 1** (Basic functionality):
1. ✅ Set up project structure (Phase 1)
2. ✅ Implement config, USCIS client, Resend client (Phase 2: items 4-6)
3. ✅ Basic main loop that fetches and emails every 5 minutes (Phase 2: item 7)
4. ✅ Bazel BUILD files (Phase 2: item 8)
5. ✅ Test locally with real credentials

**Milestone 2** (Smart notifications):
6. ✅ Add state storage (Phase 3: item 9)
7. ✅ Implement change detection (Phase 3: item 10)
8. ✅ Update main loop for smart notifications (Phase 3: item 11)
9. ✅ Add 401 error handling (Phase 3: item 12)
10. ✅ Write unit tests (Phase 3: item 13)

**Milestone 2.5** (Function Enhancement):
11. ✅ Support multiple case IDs (Phase 4: item 14)
12. ✅ Enhanced state storage with timestamps (Phase 4: items 15-16)
13. ✅ Update configuration examples (Phase 4: item 17)

**Milestone 2.75** (Browser Auto-Login with 2FA):
14. ✅ Implement BrowserClient with chromedp (Phase 5: item 18)
15. ✅ Add manual 2FA support via stdin (Phase 5 & 6)
16. ✅ Update configuration for dual auth modes (Phase 5: item 20)
17. ✅ Create CaseStatusFetcher interface in main (Phase 5: item 21)
18. ✅ Add session refresh capability (Phase 5: RefreshSession)
19. ✅ Update .env.example (Phase 5: item 22)
20. ✅ Add chromedp dependencies and test (Phase 5: items 23-24)

### 🔄 IN PROGRESS / TODO

**Milestone 3** (Production & Open Source):
- ⏳ Create Dockerfile with Chrome support (Phase 7: item 33)
- ⏳ Set up Cloud Run deployment (Phase 7: items 34-35)
- ⏳ Write documentation (Phase 8: items 37-41)
- ⏳ Add LICENSE and contributing guidelines (Phase 8: items 38-39)
- ⏳ Update .env.example template (Phase 8: item 40)
- ⏳ Final testing and release

**Note:** Phase 6 IMAP email 2FA was skipped in favor of simpler manual stdin approach

---

## Dependencies

### Go Packages
- `github.com/resend/resend-go` - Resend SDK for email notifications
- `github.com/chromedp/chromedp` v0.14.2 - Chrome DevTools Protocol for browser automation
- `github.com/chromedp/cdproto` - CDP protocol definitions
- Standard library: `net/http`, `encoding/json`, `time`, `os`, `log`, `bufio`, `context`

### System Requirements
- **For Auto-Login Mode:** Chrome/Chromium browser (automatically managed by chromedp in headless mode)
- **For Manual Cookie Mode:** Any modern web browser for cookie extraction

### Bazel Rules
- `rules_go` - Go compilation
- `gazelle` - BUILD file generation
- `rules_docker` (optional) - Container image building

### External Services
- Resend API (email delivery)
- USCIS API (case status)
- GCP Cloud Run (hosting - manual cookie mode recommended)
- GCP Secret Manager (optional, for secrets)

---

## Security Considerations

1. **Never commit credentials** - Use .gitignore, document in README
2. **Cookie/Password storage** - Use environment variables or GCP Secret Manager, never commit to git
3. **Browser mode credentials** - Username/password stored only in memory during execution
4. **Session expiration** - BrowserClient auto-refreshes, manual cookie requires user intervention
5. **API rate limiting** - 5-minute interval is conservative and respectful
6. **Input validation** - Validate case ID format before making requests
7. **Secret management** - Use GCP Secret Manager for production deployments
8. **Minimal permissions** - Cloud Run service account with least privilege
9. **2FA security** - Manual stdin prompt prevents automated code interception
10. **Browser automation** - chromedp runs in headless mode, no GUI access required

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
- Automated IMAP email 2FA (if stdin prompts become limiting)
- Playwright/Puppeteer as alternative to chromedp

---

## Build Notes

**Current Build System:**
- Primary: `go build` (works with Go 1.23.0+)
- Bazel: Configured but may have compatibility issues with Go 1.23+

**Building:**
```bash
# Using Go directly (recommended)
go build ./cmd/tracker
go build test_login.go

# Using Bazel (may need updates for Go 1.23)
bazel build //cmd/tracker:tracker
bazel run //:test_login
```

**Running:**
```bash
# Load environment variables
set -a
source .env
set +a

# Run tracker
./tracker

# Or run test login
./test_login
```
