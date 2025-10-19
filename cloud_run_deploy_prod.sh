#!/bin/bash
set -e

# USCIS Case Tracker - Cloud Run Deployment Script
# This script builds the Docker image locally and deploys to Cloud Run

# Configuration - modify these or set as environment variables
PROJECT_ID="${GCP_PROJECT_ID:-case-notification-475103}"
REGION="${GCP_REGION:-us-central1}"
SERVICE_NAME="${SERVICE_NAME:-uscis-case-tracker}"
REPOSITORY="${ARTIFACT_REGISTRY_REPO:-uscis-tracker}"
IMAGE_NAME="${REGION}-docker.pkg.dev/${PROJECT_ID}/${REPOSITORY}/${SERVICE_NAME}"

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
        run.googleapis.com \
        --project="$PROJECT_ID" || warn "Some APIs may already be enabled"

    info "APIs enabled"
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
    step "Building Docker image for AMD64 (Cloud Run)..."
    echo "Image: $IMAGE_NAME"

    # Cloud Run uses AMD64 architecture, so build for linux/amd64
    docker buildx build --platform linux/amd64 -t "$IMAGE_NAME" --load . || error "Docker build failed"

    info "Docker image built successfully"
}

# Push image to Artifact Registry
push_image() {
    step "Pushing image to Artifact Registry..."

    docker push "$IMAGE_NAME" || error "Docker push failed"

    info "Image pushed successfully"
}

# Update cloud-run.yaml with image name
update_yaml() {
    step "Updating cloud-run.yaml with image..."

    # Add timestamp to force new revision
    TIMESTAMP=$(date +%s)

    # Create temporary file with updated image
    sed "s|IMAGE_PLACEHOLDER|${IMAGE_NAME}|g" cloud-run.yaml | \
    sed "s|run.googleapis.com/execution-environment: gen2|run.googleapis.com/execution-environment: gen2\n        deploy-timestamp: \"${TIMESTAMP}\"|" > cloud-run-deploy.yaml

    info "cloud-run-deploy.yaml updated"
}

# Deploy to Cloud Run
deploy_to_cloud_run() {
    step "Deploying to Cloud Run..."

    gcloud run services replace cloud-run-deploy.yaml \
        --region="$REGION" \
        --project="$PROJECT_ID" || error "Deployment failed"

    info "Deployment completed successfully!"

    # Clean up temporary file
    rm -f cloud-run-deploy.yaml
}

# Show deployment status
show_status() {
    echo ""
    info "Deployment Summary"
    echo "  Project ID: $PROJECT_ID"
    echo "  Region: $REGION"
    echo "  Service Name: $SERVICE_NAME"
    echo "  Image: $IMAGE_NAME"
    echo ""

    step "Service Details:"
    gcloud run services describe "$SERVICE_NAME" \
        --region="$REGION" \
        --project="$PROJECT_ID" \
        --format="table(status.url,status.conditions[0].type,status.conditions[0].status)" || true

    echo ""
    info "Next Steps:"
    echo ""
    echo "1. View logs:"
    echo "   gcloud logging read \"resource.type=cloud_run_revision AND resource.labels.service_name=${SERVICE_NAME}\" \\"
    echo "     --limit 50 --format json --project=${PROJECT_ID}"
    echo ""
    echo "2. Update environment variables (edit cloud-run.yaml and redeploy):"
    echo "   ./deploy_prod.sh"
    echo ""
    echo "3. View service in console:"
    echo "   https://console.cloud.google.com/run/detail/${REGION}/${SERVICE_NAME}?project=${PROJECT_ID}"
    echo ""
}

# Main execution
main() {
    echo "========================================"
    echo "USCIS Case Tracker - Cloud Run Deployment"
    echo "========================================"
    echo ""

    check_prerequisites
    prompt_project_id
    configure_gcloud
    check_secrets
    create_artifact_registry
    configure_docker
    build_image
    push_image
    update_yaml
    deploy_to_cloud_run
    show_status
}

# Run main function
main
