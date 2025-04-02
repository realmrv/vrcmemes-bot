# Use the official Golang image as a builder stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install dependencies and build tools
# Added git and gcc for potential CGO needs if any dependency requires it, and air for live reload
RUN apk add --no-cache git build-base && \
    go install github.com/air-verse/air@latest

# Copy project files
COPY go.mod go.sum ./
# Download dependencies
RUN go mod download

# Copy the entire source code
COPY . .

# Build application using the target architecture provided by Docker BuildKit
# Output the binary into the current directory (/app)
RUN GOOS=linux GOARCH=${TARGETARCH} CGO_ENABLED=0 go build -o /app/vrcmemes-bot .

# --- Final Stage ---
# Use a minimal alpine image for the final stage
# Name this stage 'final' so docker-compose can target it
FROM alpine:latest AS final

WORKDIR /app

# Copy only the built binary from the builder stage
COPY --from=builder /app/vrcmemes-bot /app/vrcmemes-bot

# Expose port if your application listens on one (e.g., EXPOSE 8080)
# Add any other necessary files like static assets or templates here
# COPY --from=builder /app/templates ./templates
# COPY --from=builder /app/static ./static

# Run application
# The command is simply the path to the binary
CMD ["./vrcmemes-bot"] 
