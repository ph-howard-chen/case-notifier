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

## Phase 5: Containerization & Deployment

### 18. Dockerfile
- Multi-stage build:
  1. Build stage: Use Bazel to build binary
  2. Runtime stage: Minimal base image (distroless or alpine)
- Copy binary to runtime image
- Set entrypoint to `/tracker`
- No EXPOSE needed for Cloud Run (it's a worker, not a server)

### 19. Cloud Run Configuration
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

### 20. Environment Setup Documentation
**In README.md:**
- GCP project setup
- Enable Cloud Run API
- Create service account with necessary permissions
- Store secrets in Secret Manager
- Deploy command examples

### 21. .gitignore
Exclude:
- `bazel-*` directories
- `*.json` (state files)
- `.env` (local config)
- `*.log`
- Bazel cache directories

---

## Phase 6: Open Source Preparation (Milestone 3)

### 22. README.md
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

### 23. LICENSE
- Choose MIT License (most permissive for community use)
- Add copyright notice

### 24. CONTRIBUTING.md
- Code style guidelines
- How to submit issues
- Pull request process
- Testing requirements
- Branch naming convention (howardchen-* per global config)

### 25. .env.example
Template file:
```bash
USCIS_COOKIE=your_cookie_here
CASE_IDS=IOE0933798378
RESEND_API_KEY=re_xxxxxxxxxxxx
RECIPIENT_EMAIL=your-email@example.com
POLL_INTERVAL=5m
STATE_FILE_DIR=/tmp/case-tracker-states/
```

### 26. Architecture Documentation
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

For **Milestone 3** (Production & Open Source):
14. Create Dockerfile (Phase 5: item 18)
15. Set up Cloud Run deployment (Phase 5: items 19-20)
16. Write documentation (Phase 6: items 22-26)
17. Add LICENSE and contributing guidelines (Phase 6: items 23-24)
18. Create .env.example (Phase 6: item 25)
19. Final testing and release

---

## Dependencies

### Go Packages
- `github.com/resend/resend-go` - Resend SDK
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
- Automatic cookie refresh (if possible)
- State file cleanup/retention policy (e.g., keep last 30 days)
