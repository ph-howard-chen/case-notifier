#!/bin/bash
set -e

# USCIS Case Tracker - GCE Deployment Script
# This script builds the Docker image and deploys to a GCE VM instance

# Configuration - modify these or set as environment variables
PROJECT_ID="${GCP_PROJECT_ID:-case-notification-475103}"
ZONE="${GCE_ZONE:-us-central1-a}"
INSTANCE_NAME="${GCE_INSTANCE_NAME:-instance-20251018-234158}"
REPOSITORY="${ARTIFACT_REGISTRY_REPO:-uscis-tracker}"
REGION="${GCP_REGION:-us-central1}"
SERVICE_NAME="${SERVICE_NAME:-uscis-case-tracker}"
IMAGE_NAME="${REGION}-docker.pkg.dev/${PROJECT_ID}/${REPOSITORY}/${SERVICE_NAME}"
CONTAINER_NAME="uscis-tracker"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Helper functions
error() {
    echo -e "${RED}ERROR: $1${NC}" >&2
    exit 1
}

info() {
    echo -e "${GREEN}✓ $1${NC}"
}

warn() {
    echo -e "${YELLOW}⚠ $1${NC}"
}

step() {
    echo -e "${BLUE}➜ $1${NC}"
}

# Check prerequisites
check_prerequisites() {
    step "Checking prerequisites..."

    if ! command -v gcloud &> /dev/null; then
        error "gcloud CLI is not installed. Install from https://cloud.google.com/sdk/docs/install"
    fi

    if ! command -v docker &> /dev/null; then
        error "docker is not installed. Install from https://docs.docker.com/get-docker/"
    fi

    info "Prerequisites check passed"
}

# Check required secrets
check_secrets() {
    step "Checking required secrets in Secret Manager..."

    local required_secrets=("resend-api-key" "uscis-username" "uscis-password" "email-app-password")
    local missing_secrets=()

    for secret in "${required_secrets[@]}"; do
        if ! gcloud secrets describe "$secret" --project="$PROJECT_ID" &> /dev/null; then
            missing_secrets+=("$secret")
        fi
    done

    if [ ${#missing_secrets[@]} -gt 0 ]; then
        error "Missing required secrets: ${missing_secrets[*]}\n\nPlease create them first:\n  echo -n \"value\" | gcloud secrets create SECRET_NAME --data-file=- --project=$PROJECT_ID\n\nRequired secrets:\n  - resend-api-key: Resend API key for email notifications\n  - uscis-username: USCIS account username\n  - uscis-password: USCIS account password\n  - email-app-password: Gmail app password for 2FA"
    fi

    info "All required secrets exist"
}

# Prompt for project ID if not set
prompt_project_id() {
    if [ -z "$PROJECT_ID" ]; then
        echo ""
        echo "Enter your GCP Project ID:"
        read -r PROJECT_ID
        if [ -z "$PROJECT_ID" ]; then
            error "Project ID is required"
        fi
        IMAGE_NAME="${REGION}-docker.pkg.dev/${PROJECT_ID}/${REPOSITORY}/${SERVICE_NAME}"
    fi
}

# Configure gcloud
configure_gcloud() {
    step "Configuring gcloud..."
    gcloud config set project "$PROJECT_ID" || error "Failed to set project"

    info "Enabling required APIs..."
    gcloud services enable \
        artifactregistry.googleapis.com \
        compute.googleapis.com \
        secretmanager.googleapis.com \
        --project="$PROJECT_ID" || warn "Some APIs may already be enabled"

    info "APIs enabled"
}

# Check if VM instance exists
check_vm_instance() {
    step "Checking GCE instance..."

    if ! gcloud compute instances describe "$INSTANCE_NAME" \
        --zone="$ZONE" \
        --project="$PROJECT_ID" &> /dev/null; then
        error "GCE instance '$INSTANCE_NAME' not found in zone $ZONE\n\nList your instances with:\n  gcloud compute instances list --project=$PROJECT_ID"
    fi

    info "GCE instance found: $INSTANCE_NAME"
}

# Grant Secret Manager access to VM service account
setup_service_account() {
    step "Setting up service account permissions..."

    # Get the service account used by the VM
    local vm_sa=$(gcloud compute instances describe "$INSTANCE_NAME" \
        --zone="$ZONE" \
        --project="$PROJECT_ID" \
        --format="get(serviceAccounts[0].email)")

    if [ -z "$vm_sa" ]; then
        warn "VM is using default Compute Engine service account"
        local project_number=$(gcloud projects describe "$PROJECT_ID" --format="value(projectNumber)")
        vm_sa="${project_number}-compute@developer.gserviceaccount.com"
    fi

    info "VM Service Account: $vm_sa"

    # Grant Secret Manager access
    step "Granting Secret Manager access..."
    gcloud projects add-iam-policy-binding "$PROJECT_ID" \
        --member="serviceAccount:${vm_sa}" \
        --role="roles/secretmanager.secretAccessor" \
        --condition=None &> /dev/null || warn "IAM binding may already exist"

    info "Secret Manager access granted"
}

# Create Artifact Registry repository if it doesn't exist
create_artifact_registry() {
    step "Checking Artifact Registry repository..."

    if gcloud artifacts repositories describe "$REPOSITORY" \
        --location="$REGION" \
        --project="$PROJECT_ID" &> /dev/null; then
        info "Artifact Registry repository already exists"
    else
        info "Creating Artifact Registry repository..."
        gcloud artifacts repositories create "$REPOSITORY" \
            --repository-format=docker \
            --location="$REGION" \
            --description="USCIS Case Tracker Docker images" \
            --project="$PROJECT_ID" || error "Failed to create repository"
        info "Repository created"
    fi
}

# Configure Docker for Artifact Registry
configure_docker() {
    step "Configuring Docker authentication..."
    gcloud auth configure-docker "${REGION}-docker.pkg.dev" --quiet || error "Failed to configure Docker"
    info "Docker configured for Artifact Registry"
}

# Build Docker image locally
build_image() {
    step "Building Docker image for AMD64 (GCE)..."
    echo "Image: $IMAGE_NAME"

    # GCE uses AMD64 architecture, so build for linux/amd64
    docker buildx build --platform linux/amd64 -t "$IMAGE_NAME" --load . || error "Docker build failed"

    info "Docker image built successfully"
}

# Push image to Artifact Registry
push_image() {
    step "Pushing image to Artifact Registry..."

    docker push "$IMAGE_NAME" || error "Docker push failed"

    info "Image pushed successfully"
}

# Deploy to GCE VM
deploy_to_gce() {
    step "Deploying to GCE instance..."

    # Fetch secrets from Secret Manager
    step "Fetching secrets..."
    local resend_api_key=$(gcloud secrets versions access latest --secret=resend-api-key --project="$PROJECT_ID")
    local uscis_username=$(gcloud secrets versions access latest --secret=uscis-username --project="$PROJECT_ID")
    local uscis_password=$(gcloud secrets versions access latest --secret=uscis-password --project="$PROJECT_ID")
    local email_password=$(gcloud secrets versions access latest --secret=email-app-password --project="$PROJECT_ID")

    info "Secrets fetched"

    # Create deployment script to run on VM
    step "Creating deployment script..."
    cat > /tmp/deploy-container.sh <<'EOF_OUTER'
#!/bin/bash
set -e

# Install Docker if not already installed
if ! command -v docker &> /dev/null; then
    echo "Docker not found, installing..."
    sudo apt-get update
    sudo apt-get install -y docker.io
    sudo systemctl start docker
    sudo systemctl enable docker
    sudo usermod -aG docker $USER
    echo "Docker installed successfully"
else
    echo "Docker already installed"
fi

echo "Configuring Docker authentication on VM..."
gcloud auth configure-docker REGION_PLACEHOLDER --quiet

# Use gcloud to pull the image with proper service account credentials
echo "Pulling latest image using gcloud docker helper..."
sudo gcloud auth configure-docker REGION_PLACEHOLDER --quiet
sudo -E docker pull IMAGE_PLACEHOLDER

echo "Stopping and removing old container (if exists)..."
sudo docker stop CONTAINER_PLACEHOLDER 2>/dev/null || true
sudo docker rm CONTAINER_PLACEHOLDER 2>/dev/null || true

echo "Starting container..."
sudo docker run -d \
  --name CONTAINER_PLACEHOLDER \
  --restart unless-stopped \
  -p 8080:8080 \
  -e CASE_IDS="IOE0933798378" \
  -e RECIPIENT_EMAIL="ph.howard.chen@gmail.com" \
  -e POLL_INTERVAL="15m" \
  -e STATE_FILE_DIR="/tmp/case-tracker-states/" \
  -e AUTO_LOGIN="true" \
  -e EMAIL_IMAP_SERVER="imap.gmail.com:993" \
  -e EMAIL_USERNAME="gtoshiba011@gmail.com" \
  -e EMAIL_2FA_SENDER="MyAccount@uscis.dhs.gov" \
  -e EMAIL_2FA_TIMEOUT="10m" \
  -e RESEND_API_KEY='RESEND_API_KEY_PLACEHOLDER' \
  -e USCIS_USERNAME='USCIS_USERNAME_PLACEHOLDER' \
  -e USCIS_PASSWORD='USCIS_PASSWORD_PLACEHOLDER' \
  -e EMAIL_PASSWORD='EMAIL_PASSWORD_PLACEHOLDER' \
  IMAGE_PLACEHOLDER

echo "Container started successfully"
sudo docker ps -a --filter name=CONTAINER_PLACEHOLDER
EOF_OUTER

    # Replace placeholders in the script
    sed -i.bak \
        -e "s|REGION_PLACEHOLDER|${REGION}-docker.pkg.dev|g" \
        -e "s|IMAGE_PLACEHOLDER|${IMAGE_NAME}|g" \
        -e "s|CONTAINER_PLACEHOLDER|${CONTAINER_NAME}|g" \
        -e "s|RESEND_API_KEY_PLACEHOLDER|${resend_api_key}|g" \
        -e "s|USCIS_USERNAME_PLACEHOLDER|${uscis_username}|g" \
        -e "s|USCIS_PASSWORD_PLACEHOLDER|${uscis_password}|g" \
        -e "s|EMAIL_PASSWORD_PLACEHOLDER|${email_password}|g" \
        /tmp/deploy-container.sh
    rm -f /tmp/deploy-container.sh.bak

    # Copy script to VM and execute
    step "Copying deployment script to VM..."
    gcloud compute scp /tmp/deploy-container.sh "${INSTANCE_NAME}:/tmp/" \
        --zone="$ZONE" \
        --project="$PROJECT_ID" || error "Failed to copy script"

    step "Executing deployment on VM..."
    gcloud compute ssh "$INSTANCE_NAME" \
        --zone="$ZONE" \
        --project="$PROJECT_ID" \
        --command="chmod +x /tmp/deploy-container.sh && /tmp/deploy-container.sh" || error "Deployment failed"

    # Clean up local script
    rm -f /tmp/deploy-container.sh

    info "Deployment completed successfully!"
}

# Show deployment status
show_status() {
    echo ""
    info "Deployment Summary"
    echo "  Project ID: $PROJECT_ID"
    echo "  Zone: $ZONE"
    echo "  Instance: $INSTANCE_NAME"
    echo "  Image: $IMAGE_NAME"
    echo ""

    step "Container Status:"
    gcloud compute ssh "$INSTANCE_NAME" \
        --zone="$ZONE" \
        --project="$PROJECT_ID" \
        --command="sudo docker ps --filter name=${CONTAINER_NAME}" || true
    echo ""

    info "Next Steps:"
    echo ""
    echo "1. View logs:"
    echo "   gcloud compute ssh $INSTANCE_NAME --zone=$ZONE --project=$PROJECT_ID --command='sudo docker logs -f ${CONTAINER_NAME}'"
    echo ""
    echo "2. Check container status:"
    echo "   gcloud compute ssh $INSTANCE_NAME --zone=$ZONE --project=$PROJECT_ID --command='sudo docker ps -a --filter name=${CONTAINER_NAME}'"
    echo ""
    echo "3. Stop container:"
    echo "   gcloud compute ssh $INSTANCE_NAME --zone=$ZONE --project=$PROJECT_ID --command='sudo docker stop ${CONTAINER_NAME}'"
    echo ""
    echo "4. Restart container:"
    echo "   gcloud compute ssh $INSTANCE_NAME --zone=$ZONE --project=$PROJECT_ID --command='sudo docker start ${CONTAINER_NAME}'"
    echo ""
    echo "5. SSH into VM:"
    echo "   gcloud compute ssh $INSTANCE_NAME --zone=$ZONE --project=$PROJECT_ID"
    echo ""
}

# Main execution
main() {
    echo "========================================"
    echo "USCIS Case Tracker - GCE Deployment"
    echo "========================================"
    echo ""

    check_prerequisites
    prompt_project_id
    configure_gcloud
    check_secrets
    check_vm_instance
    setup_service_account
    create_artifact_registry
    configure_docker
    build_image
    push_image
    deploy_to_gce
    show_status
}

# Run main function
main
