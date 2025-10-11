#!/bin/bash

# Load environment variables from .env file
if [ -f .env ]; then
    echo "Loading environment variables from .env..."
    export $(cat .env | grep -v '^#' | xargs)
else
    echo "Error: .env file not found. Please copy .env.example to .env and fill in your credentials."
    exit 1
fi

# Check required environment variables
required_vars=("USCIS_COOKIE" "CASE_ID" "RESEND_API_KEY" "RECIPIENT_EMAIL")
for var in "${required_vars[@]}"; do
    if [ -z "${!var}" ]; then
        echo "Error: $var is not set in .env"
        exit 1
    fi
done

echo "Starting USCIS Case Tracker..."
echo "Configuration:"
echo "  Case ID: $CASE_ID"
echo "  Recipient: $RECIPIENT_EMAIL"
echo "  Poll Interval: ${POLL_INTERVAL:-5m}"
echo ""

# Run the tracker
bazel run //cmd/tracker:tracker
