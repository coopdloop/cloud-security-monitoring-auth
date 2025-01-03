# Dockerfile
FROM golang:1.23-alpine AS builder

WORKDIR /app
# Required
COPY go.mod go.sum ./
# Required
RUN go mod download
# Build phase copy all
COPY . .

# Build arguments for environment variables
ARG AUTH0_CLIENT_ID
ARG AUTH0_DOMAIN
ARG AUTH0_CLIENT_SECRET
ARG AUTH0_CALLBACK_URL
ARG DATABASE_URL

# Set environment variables from build args
ENV AUTH0_CLIENT_ID=$AUTH0_CLIENT_ID
ENV AUTH0_DOMAIN=$AUTH0_DOMAIN
ENV AUTH0_CLIENT_SECRET=$AUTH0_CLIENT_SECRET
ENV AUTH0_CALLBACK_URL=$AUTH0_CALLBACK_URL
ENV DATABASE_URL=$DATABASE_URL

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o main ./cmd/server/main.go
# RUN go build -o /app/server cmd/server/main.go

FROM alpine:latest

WORKDIR /app
# Copy server binary
# COPY --from=builder /app/server .
# Copy the pre-built binary file from the previous stage
COPY --from=builder /app/main .
COPY --from=builder /app/api ./api
COPY --from=builder /app/internal ./internal

# Copy all sql schemas
COPY sql/schema /app/sql/schema
# Copy frontend files
COPY frontend/public /app/frontend/public

EXPOSE 3000
CMD ["./main"]
