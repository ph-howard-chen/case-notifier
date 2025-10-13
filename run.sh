#!/bin/bash

# Load environment variables from .env file
if [ -f .env ]; then
    echo "Loading environment variables from .env..."
    set -a
    source .env
    set +a
else
    echo "Error: .env file not found. Please copy .env.example to .env and fill in your credentials."
    exit 1
fi

# Check authentication mode
if [ "${AUTO_LOGIN}" = "true" ]; then
    echo "Authentication mode: Auto-login (username/password)"
    required_vars=("USCIS_USERNAME" "USCIS_PASSWORD" "CASE_IDS" "RESEND_API_KEY" "RECIPIENT_EMAIL")
else
    echo "Authentication mode: Manual cookie"
    required_vars=("USCIS_COOKIE" "CASE_IDS" "RESEND_API_KEY" "RECIPIENT_EMAIL")
fi

# Check required environment variables
for var in "${required_vars[@]}"; do
    if [ -z "${!var}" ]; then
        echo "Error: $var is not set in .env"
        exit 1
    fi
done

echo "Starting USCIS Case Tracker..."
echo "Configuration:"
echo "  Case IDs: $CASE_IDS"
echo "  Recipient: $RECIPIENT_EMAIL"
echo "  Poll Interval: ${POLL_INTERVAL:-5m}"
echo ""

# Always rebuild the tracker
echo "Building tracker..."
go build -o tracker ./cmd/tracker
if [ $? -ne 0 ]; then
    echo "Error: Build failed"
    exit 1
fi
echo "Build successful"

# Run the tracker
./tracker
