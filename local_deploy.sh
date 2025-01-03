#!/bin/bash

export PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH"

# Strict mode: exit on any error, unset variable, or pipeline failure
set -euo pipefail

# Ensure AWS CLI and Docker are installed
command -v aws >/dev/null 2>&1 || { echo "AWS CLI is not installed. Aborting."; exit 1; }
command -v docker >/dev/null 2>&1 || {
    echo "Docker is not installed or not in PATH. Please check your Docker installation.";
    exit 1;
}

# Load environment variables
ENV_FILE=${ENV_FILE:-.env}

# Check if .env file exists
if [ ! -f "$ENV_FILE" ]; then
    echo "Error: $ENV_FILE file not found. Please create it based on .env.example"
    exit 1
fi

# Validate required environment variables
REQUIRED_VARS=(
    "AUTH0_CLIENT_ID"
    "AUTH0_DOMAIN"
    "AUTH0_CLIENT_SECRET"
    "DATABASE_URL"
)

MISSING_VARS=()

# Check for required variables
for var in "${REQUIRED_VARS[@]}"; do
    if ! grep -q "^$var=" "$ENV_FILE"; then
        echo "Error: $var is not set in $ENV_FILE"
        MISSING_VARS+=("$var")
    fi
done

# Exit if any required variables are missing
if [ ${#MISSING_VARS[@]} -ne 0 ]; then
    echo "Missing required environment variables: ${MISSING_VARS[*]}"
    echo "Please update your $ENV_FILE file"
    exit 1
fi

# Set default variables with fallback
AWS_REGION=${AWS_REGION:-us-east-1}
ECR_REPOSITORY=${ECR_REPOSITORY:-qa-backend-repo}
IMAGE_TAG=${IMAGE_TAG:-latest}

# Prepare build arguments from .env file
BUILD_ARGS=""
while IFS= read -r line; do
    # Skip comments and empty lines
    [[ "$line" =~ ^\s*#|^\s*$ ]] && continue

    # Add each non-empty, non-comment line as a build argument
    BUILD_ARGS+="--build-arg ${line} "
done < "$ENV_FILE"

# Verify AWS authentication
echo "Verifying AWS authentication..."
AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)

# Login to Amazon ECR
echo "Logging into Amazon ECR..."
aws ecr get-login-password --region "$AWS_REGION" | \
    docker login --username AWS --password-stdin "$AWS_ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com"

# Build Docker image with environment variables
echo "Building Docker image..."
docker build $BUILD_ARGS -t "$ECR_REPOSITORY:$IMAGE_TAG" .

# Tag image for ECR
FULL_IMAGE_NAME="$AWS_ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com/$ECR_REPOSITORY:$IMAGE_TAG"
docker tag "$ECR_REPOSITORY:$IMAGE_TAG" "$FULL_IMAGE_NAME"

# Push to ECR (optional)
read -p "Push image to ECR? (y/n) " PUSH_TO_ECR
if [[ $PUSH_TO_ECR == "y" ]]; then
    echo "Pushing image to ECR..."
    docker push "$FULL_IMAGE_NAME"
fi

# Local testing
echo "Running container locally..."
# docker run --rm -it \
#     -p 3000:3000 \
#     --env-file "$ENV_FILE" \
#     -e ENV=local \
#     "$ECR_REPOSITORY:$IMAGE_TAG"
docker compose -f docker-compose.dev.yml up --build

# Optional: Deploy to ECS
read -p "Deploy to ECS? (y/n) " DEPLOY_ECS
if [[ $DEPLOY_ECS == "y" ]]; then
    echo "Updating ECS service..."
    aws ecs update-service \
        --cluster qa-backend-cluster \
        --service qa-backend-service \
        --task-definition qa-backend-task \
        --force-new-deployment
fi

echo "Deployment process completed."
