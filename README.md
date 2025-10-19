# USCIS Case Tracker

Automated monitoring service for USCIS case status changes with email notifications.

## Features

- 📊 **Smart Change Detection**: Only sends notifications when case status actually changes
- 📧 **Email Notifications**: HTML-formatted emails via Resend API
- 🔐 **Browser Automation**: Auto-login with chromedp (production-ready)
- 📦 **Multi-Case Support**: Monitor multiple cases simultaneously
- 💾 **State Persistence**: Timestamped state files for historical tracking
- 🔄 **Automated 2FA**: Optional IMAP email fetching for verification codes
- ☁️ **Cloud-Ready**: Containerized with Docker, deploy to GCE (FREE) or Cloud Run
- 💰 **Cost Effective**: Run completely FREE on GCE e2-micro or locally with Docker

## Prerequisites

### For Local Development
- Go 1.23+
- Bazel 6.4.0+ (optional, can use `go build`)
- Docker (for containerization)

### For Cloud Deployment
- Google Cloud Platform account
- gcloud CLI installed
- Docker installed
- Resend API account

## Quick Start - Local Development

### 1. Clone and Setup

```bash
git clone https://github.com/ph-howard-chen/case-tracker.git
cd case-tracker
```

### 2. Configure Environment

Copy `.env.example` to `.env` and configure:

```bash
cp .env.example .env
```

**IMPORTANT:** All user-specific configuration (emails, case IDs, credentials) should be set in `.env`. The deployment scripts automatically read from these environment variables. You do NOT need to edit any scripts or YAML files directly.

**Minimum required configuration (for local testing):**

```bash
# Manual cookie mode (local testing only)
AUTO_LOGIN=false
USCIS_COOKIE='_myuscis_session_rx=your_cookie_here'

# Case tracking
CASE_IDS=IOE0123456789,IOE0987654321
RESEND_API_KEY=re_xxxxxxxxxxxx
RECIPIENT_EMAIL=your-email@example.com
```

### 3. Build and Run

**Option A: Using Go directly (recommended)**

```bash
go build -o tracker ./cmd/tracker
./tracker
```

**Option B: Using Bazel**

```bash
bazel build //cmd/tracker:tracker
bazel-bin/cmd/tracker/tracker_/tracker
```

**Option C: Using the helper script**

```bash
./deploy_dev.sh
```

## Getting Your USCIS Cookie (Local Development Only)

**⚠️ IMPORTANT: Manual cookie mode ONLY works for local development using `./deploy_dev.sh`. It does NOT work for Cloud Run production deployment due to AWS WAF and Akamai bot detection.**

For local testing with manual cookie mode:

1. Login to https://my.uscis.gov in your browser
2. Open Developer Tools (F12)
3. Go to Network tab
4. Refresh the page
5. Click any request to `my.uscis.gov`
6. Look for Cookie header
7. Copy the value of `_myuscis_session_rx` cookie
8. Set it in `.env`: `USCIS_COOKIE='_myuscis_session_rx=...'`
9. Run locally: `./deploy_dev.sh`

**Why cookies don't work in production:**
- AWS WAF and Akamai require additional browser fingerprinting tokens
- These tokens can't be extracted and reused outside a real browser session
- Production deployments MUST use browser automation (auto-login mode)

## Production Deployment Options

Choose the deployment option that best fits your needs:

| Option | Cost | Complexity | Best For |
|--------|------|------------|----------|
| **Local Docker** | ✅ **FREE** | Low | Running on your own computer 24/7 |
| **GCE (Compute Engine)** | ✅ **FREE** (e2-micro) | Medium | Cloud deployment with free tier |
| **Cloud Run** | 💰 ~$5-10/month | Low | Serverless, auto-scaling (not free) |

---

## Option 1: Local Docker Deployment (FREE)

Run the tracker on your local computer using Docker.

### Prerequisites
- Docker installed
- Computer running 24/7 (or as needed)

### Quick Start

```bash
# Build and run locally
./deploy_dev.sh
```

**Pros:**
- ✅ Completely FREE
- ✅ Full control
- ✅ Easy to debug

**Cons:**
- ❌ Computer must run 24/7
- ❌ Uses your internet connection

---

## Option 2: GCE Deployment (FREE with Free Tier)

Deploy to Google Compute Engine using the free e2-micro instance.

### Prerequisites

1. **GCP Project**: Create or select a project
2. **Install gcloud CLI**: https://cloud.google.com/sdk/docs/install
3. **Authenticate**: `gcloud auth login`
4. **USCIS Account**: Valid username and password
5. **Gmail Account**: For receiving 2FA codes (requires IMAP app password)
6. **GCE Instance**: Create a free e2-micro VM instance

### Step 1: Configure and Load Environment Variables
Edit `.env` and set your configuration:

```bash
cp .env.example .env
# Edit .env with your settings
```

Load environment variables from .env
```bash
set -a && source .env && set +a
```

**IMPORTANT:** All configuration is now centralized in `.env`. You do NOT need to edit `deploy_gce.sh` directly.

### Step 2: Create Secrets in Secret Manager

```bash
# Required: Resend API key for email notifications
echo -n "re_xxxxxxxxxxxx" | gcloud secrets create resend-api-key \
  --data-file=- --project=your-gcp-project-id

# Required: USCIS login credentials (browser automation)
echo -n "your_uscis_username" | gcloud secrets create uscis-username \
  --data-file=- --project=your-gcp-project-id

echo -n "your_uscis_password" | gcloud secrets create uscis-password \
  --data-file=- --project=your-gcp-project-id

# Required: Gmail app password for 2FA code retrieval
echo -n "your_gmail_app_password" | gcloud secrets create email-app-password \
  --data-file=- --project=your-gcp-project-id
```

**How to get Gmail app password:**
1. Go to https://myaccount.google.com/apppasswords
2. Create a new app password for "Mail"
3. Use this password (NOT your regular Gmail password)

**How to check/update secrets:**
```bash
# List all secrets
gcloud secrets list --project=your-gcp-project-id

# View a secret value
gcloud secrets versions access latest --secret=uscis-username --project=your-gcp-project-id
gcloud secrets versions access latest --secret=uscis-password --project=your-gcp-project-id

# Update a secret (add new version)
echo -n "new_password" | gcloud secrets versions add uscis-password \
  --data-file=- --project=your-gcp-project-id
```

### Step 3: Create GCE VM Instance

Create a free e2-micro VM instance:

```bash
# Create e2-micro instance in us-central1 (free tier eligible)
gcloud compute instances create your-vm-name \
  --project=your-gcp-project-id \
  --zone=your-gcp-zone \
  --machine-type=e2-micro \
  --network-interface=network-tier=PREMIUM,stack-type=IPV4_ONLY,subnet=default \
  --maintenance-policy=MIGRATE \
  --provisioning-model=STANDARD \
  --scopes=https://www.googleapis.com/auth/cloud-platform \
  --tags=http-server,https-server \
  --create-disk=auto-delete=yes,boot=yes,device-name=your-vm-name,image=projects/debian-cloud/global/images/debian-12-bookworm-v20241009,mode=rw,size=10,type=pd-balanced \
  --no-shielded-secure-boot \
  --shielded-vtpm \
  --shielded-integrity-monitoring \
  --labels=goog-ec-src=vm_add-gcloud \
  --reservation-affinity=any
```

### Step 4: Grant Secret Manager Access

Grant the VM service account permission to access secrets:

```bash
# Get your project number
PROJECT_NUMBER=$(gcloud projects describe your-gcp-project-id --format="value(projectNumber)")

# Grant Secret Manager access to default compute service account
gcloud projects add-iam-policy-binding your-gcp-project-id \
  --member="serviceAccount:${PROJECT_NUMBER}-compute@developer.gserviceaccount.com" \
  --role="roles/secretmanager.secretAccessor"
```

**What this does:** Allows the GCE VM to read the secrets you created in Step 1.

### Step 5: Deploy

```bash
./deploy_gce.sh
```

The script will:
1. ✓ Check prerequisites
2. ✓ Enable required APIs
3. ✓ Check VM instance exists
4. ✓ Grant service account permissions
5. ✓ Create Artifact Registry repository
6. ✓ Build Docker image (AMD64 for GCE)
7. ✓ Push to Artifact Registry
8. ✓ Deploy container to GCE VM
9. ✓ Auto-install Docker if needed
10. ✓ Configure auto-restart

### Step 6: Monitor

```bash
# View container logs
gcloud compute ssh your-vm-name \
  --zone=your-gcp-zone \
  --project=your-gcp-project-id \
  --command='sudo docker logs -f uscis-tracker'

# Check container status
gcloud compute ssh your-vm-name \
  --zone=your-gcp-zone \
  --project=your-gcp-project-id \
  --command='sudo docker ps -a --filter name=uscis-tracker'

# SSH into VM (for debugging)
gcloud compute ssh your-vm-name \
  --zone=your-gcp-zone \
  --project=your-gcp-project-id
```

### Step 7: Stop or Delete

```bash
# Stop container (keeps VM running)
gcloud compute ssh your-vm-name \
  --zone=your-gcp-zone \
  --project=your-gcp-project-id \
  --command='sudo docker stop uscis-tracker'

# Restart container
gcloud compute ssh your-vm-name \
  --zone=your-gcp-zone \
  --project=your-gcp-project-id \
  --command='sudo docker start uscis-tracker'

# Stop VM instance (saves money if not using)
gcloud compute instances stop your-vm-name \
  --zone=your-gcp-zone \
  --project=your-gcp-project-id

# Delete VM instance completely
gcloud compute instances delete your-vm-name \
  --zone=your-gcp-zone \
  --project=your-gcp-project-id \
  --quiet
```

**Pros:**
- ✅ Completely FREE (e2-micro free tier)
- ✅ No cluster management fees
- ✅ Full control over VM
- ✅ Easy to debug with SSH access

**Cons:**
- ❌ Manual VM management
- ❌ No auto-scaling
- ❌ Requires more setup than Cloud Run

### Troubleshooting (GCE)

**Problem: Authentication failures**

If you see login failure in the logs, the service automatically exits to prevent account lockout:

```bash
# Check container status
gcloud compute ssh your-vm-name --zone=your-gcp-zone \
  --command='sudo docker ps -a --filter name=uscis-tracker'

# View logs to see the error
gcloud compute ssh your-vm-name --zone=your-gcp-zone \
  --command='sudo docker logs --tail=50 uscis-tracker'
```

**What causes this:**
- Incorrect USCIS username or password
- Account locked due to too many failed attempts
- Email password incorrect (for 2FA)

**How it protects you:**
- Service exits with error code 1 on first auth failure
- Docker does NOT restart on exit code 1 (only restarts on normal exit)
- Prevents repeated failed login attempts that could lock your USCIS account

**To retry after fixing credentials:**
1. Update secrets in Secret Manager: `echo -n "new_password" | gcloud secrets versions add uscis-password --data-file=- --project=your-gcp-project-id`
2. Redeploy: `./deploy_gce.sh` (automatically uses new credentials)

**Problem: Docker not installed on VM**
```bash
# SSH into VM and install Docker manually
gcloud compute ssh your-vm-name --zone=your-gcp-zone
sudo apt-get update && sudo apt-get install -y docker.io
sudo systemctl start docker
sudo systemctl enable docker
```

**Problem: Container won't start**
```bash
# Check Docker logs
gcloud compute ssh your-vm-name --zone=your-gcp-zone \
  --command='sudo docker logs uscis-tracker'

# Check if container is running
gcloud compute ssh your-vm-name --zone=your-gcp-zone \
  --command='sudo docker ps -a'
```

**Problem: Can't pull image from Artifact Registry**
```bash
# Configure Docker authentication on VM
gcloud compute ssh your-vm-name --zone=your-gcp-zone \
  --command='gcloud auth configure-docker us-central1-docker.pkg.dev --quiet'
```

**Problem: Secret Manager access denied**

See "Step 2: Create Secrets in Secret Manager"

**Problem: Memory Issues (Auto-login mode)**

Upgrade to a larger instance type (but this loses free tier):

```bash
# Stop the instance first
gcloud compute instances stop your-vm-name --zone=your-gcp-zone

# Change machine type
gcloud compute instances set-machine-type your-vm-name \
  --machine-type=e2-small \
  --zone=your-gcp-zone

# Start the instance
gcloud compute instances start your-vm-name --zone=your-gcp-zone
```

**Note**: e2-small is NOT free tier. Consider optimizing your application instead.

---

## Option 3: Cloud Run Deployment (~$5-10/month)

Deploy to Google Cloud Run for serverless, auto-scaling deployment.

### Prerequisites

1. **GCP Project**: Create or select a project
2. **Install gcloud CLI**: https://cloud.google.com/sdk/docs/install
3. **Authenticate**: `gcloud auth login`
4. **USCIS Account**: Valid username and password
5. **Gmail Account**: For receiving 2FA codes (requires IMAP app password)

### Step 1: Configure and Load Environment Variables
Same as Option 2

### Step 2: Create Secrets in Secret Manager
Same as Option 2

### Step 3: Grant Secret Manager Access

```bash
# Grant Secret Manager access to Cloud Run service account
gcloud projects add-iam-policy-binding your-gcp-project-id \
  --member="default@${GCP_PROJECT_ID}.iam.gserviceaccount.com" \
  --role="roles/secretmanager.secretAccessor"


gcloud projects add-iam-policy-binding your-gcp-project-id \
  --member="default@${GCP_PROJECT_ID}.iam.gserviceaccount.com" \
  --role="roles/run.serviceAgent"
```

### Step 4: Deploy
```bash
./cloud_run_deploy_prod.sh
```

The script will:
1. ✓ Check prerequisites
2. ✓ Enable required APIs
3. ✓ Create Artifact Registry repository
4. ✓ Build Docker image
5. ✓ Push to Artifact Registry
6. ✓ Deploy to Cloud Run

### Step 5: Monitor

```bash
# View logs
gcloud logging read \
  "resource.type=cloud_run_revision AND resource.labels.service_name=uscis-case-tracker" \
  --limit 50 --project=your-gcp-project-id

# View service status
gcloud run services describe uscis-case-tracker \
  --region=us-central1 --project=your-gcp-project-id
```

### Step 6: Stop or Delete the Service

```bash
# Stop the service (delete it completely)
gcloud run services delete uscis-case-tracker \
  --region=us-central1 \
  --project=your-gcp-project-id \
  --quiet

# Or update traffic to 0 to pause without deleting
gcloud run services update-traffic uscis-case-tracker \
  --to-revisions=LATEST=0 \
  --region=us-central1 \
  --project=your-gcp-project-id
```

**Pros:**
- ✅ Serverless (no VM management)
- ✅ Auto-scaling
- ✅ Easy deployment with single script

**Cons:**
- ❌ NOT free (~$5-10/month with min-instances=1)
- ❌ Less control than VM
- ❌ Cannot use min-instances=0 (would stop polling)

**Important**: The tracker requires `min-instances=1` in `cloud-run.yaml` because:
- It's a worker service that needs to run continuously
- It uses a `time.Ticker` to poll USCIS every 5 minutes
- If Cloud Run scales to 0, the container shuts down and polling stops
- Setting min-instances=0 would break the entire service

**Note**: Deleting the service stops all instances and billing. You can redeploy anytime with `./deploy_cloud_run.sh`.

### Troubleshooting (Cloud Run)

**Problem: Authentication failures**

Same as Option 2 (GCE). You will receive an email notification when authentication fails.

**To retry after fixing credentials:**
1. Update secrets in Secret Manager: `echo -n "new_password" | gcloud secrets versions add uscis-password --data-file=- --project=your-gcp-project-id`
2. Redeploy: `./deploy_cloud_run.sh` (automatically uses new credentials)

**Problem: Deployment fails**

```bash
# Check service account permissions
gcloud projects get-iam-policy your-gcp-project-id

# Verify secrets exist
gcloud secrets list --project=your-gcp-project-id

# Check service logs for errors
gcloud logging read \
  "resource.type=cloud_run_revision AND severity>=ERROR" \
  --limit 50 --project=your-gcp-project-id
```

**Problem: Secret Manager access denied**

See "Step 3: Grant Secret Manager Access"

**Problem: Memory Issues (Auto-login mode)**

Increase memory in `cloud-run.yaml`:

```yaml
resources:
  limits:
    memory: 1Gi  # Increase from 512Mi if needed
```

Then redeploy: `./deploy_cloud_run.sh`

---

## Configuration

### Environment Variables

#### Required for All Deployments

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `CASE_IDS` | Yes | - | Comma-separated case IDs |
| `RESEND_API_KEY` | Yes | - | Resend API key |
| `RECIPIENT_EMAIL` | Yes | - | Email for notifications |
| `POLL_INTERVAL` | No | 5m | How often to check status |
| `STATE_FILE_DIR` | No | /tmp/case-tracker-states/ | Directory for state files |

#### Local Development Only (Manual Cookie Mode)

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `AUTO_LOGIN` | No | false | Set to false for cookie mode |
| `USCIS_COOKIE` | Yes | - | Session cookie from browser (local only) |

**⚠️ Manual cookie mode does NOT work in Cloud Run production!**

#### GCE and Cloud Run Production (Browser Automation)

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `AUTO_LOGIN` | Yes | - | Must be `true` for production |
| `USCIS_USERNAME` | Yes | - | USCIS account username |
| `USCIS_PASSWORD` | Yes | - | USCIS account password |
| `EMAIL_IMAP_SERVER` | Yes | - | IMAP server (e.g., imap.gmail.com:993) |
| `EMAIL_USERNAME` | Yes | - | Gmail for receiving 2FA codes |
| `EMAIL_PASSWORD` | Yes | - | Gmail app password (NOT regular password) |
| `EMAIL_2FA_SENDER` | No | MyAccount@uscis.dhs.gov | USCIS 2FA sender email |
| `EMAIL_2FA_TIMEOUT` | No | 5m | Max wait time for 2FA email |

## Cost Optimization

### Free Tier Limits (GCP)
- **GCE (Compute Engine)**: 1 e2-micro instance FREE per month (US regions only)
- **Cloud Run**: First 2M requests/month FREE, but min-instances=1 costs ~$5-10/month
- **Artifact Registry**: 0.5 GB storage FREE
- **Secret Manager**: 6 active secret versions FREE
- **Network Egress**: 1 GB FREE per month

### Recommended Configuration by Option

**GCE (FREE):**
- Use e2-micro instance (730 hours/month FREE)
- Use us-west1, us-central1, or us-east1 zones
- 30 GB standard persistent disk FREE
- Completely FREE if you stay within limits!

**Cloud Run (~$5-10/month):**
- **min-instances=1** is REQUIRED in `cloud-run.yaml` to keep the polling service alive
  - Setting to 0 would shut down the container between requests
  - Your tracker needs to run continuously to poll USCIS every 5 minutes
  - If it shuts down, polling stops and notifications won't work
- Use **1Gi memory** for browser automation (required for Chromium)
- Adjust `POLL_INTERVAL` to reduce costs (e.g., 15m instead of 5m)

**Local Docker (FREE):**
- No cloud costs
- Just your electricity and internet

### Cost Calculator
Estimate costs at: https://cloud.google.com/products/calculator

### FREE Alternative: Run Locally
Running locally on your computer is completely FREE:

```bash
# Run in background
nohup ./tracker > tracker.log 2>&1 &

# Check status
ps aux | grep tracker

# View logs
tail -f tracker.log
```

## Architecture

### Authentication Modes

| Mode | Pros | Cons | Recommended For |
|------|------|------|-----------------|
| **Manual Cookie** | ✅ Lightweight<br>✅ Fast startup<br>✅ Quick local testing | ❌ Manual cookie refresh needed<br>❌ Cookies expire frequently<br>❌ No 2FA automation | Local development only |
| **Browser Automation** | ✅ Auto-login<br>✅ Session auto-refresh<br>✅ 2FA support<br>✅ Production-ready | ❌ Larger image (~1GB)<br>❌ Higher memory (1Gi)<br>❌ Includes Chromium | Cloud Run production |

### Key Components

```
┌─────────────────┐
│   Main Loop     │ Poll every 5 minutes
└────────┬────────┘
         │
         ├─► ┌──────────────┐
         │   │ USCIS Client │ Fetch case status
         │   └──────────────┘
         │
         ├─► ┌──────────────┐
         │   │   Storage    │ Load/save state
         │   └──────────────┘
         │
         ├─► ┌──────────────┐
         │   │   Detector   │ Compare changes
         │   └──────────────┘
         │
         └─► ┌──────────────┐
             │   Notifier   │ Send email
             └──────────────┘
```

## Development

### Run Tests

```bash
# Unit tests
go test ./...

# Specific package
go test ./internal/uscis -v

# With coverage
go test -cover ./...
```

### Build with Bazel

```bash
# Update BUILD files
bazel run //:gazelle

# Build
bazel build //cmd/tracker:tracker

# Test
bazel test //...
```

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

MIT License - See [LICENSE](LICENSE) file for details

## Support

For issues and questions:
- Open an issue on GitHub
- Check existing issues for solutions

## Acknowledgments

- Built with [chromedp](https://github.com/chromedp/chromedp) for browser automation
- Email notifications via [Resend](https://resend.com)
- IMAP email client via [go-imap](https://github.com/emersion/go-imap)
- Deployed on [Google Cloud Run](https://cloud.google.com/run) or [Google Compute Engine](https://cloud.google.com/compute)
