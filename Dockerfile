FROM golang:1.24-alpine

WORKDIR /app

# Install dependencies and build tools
RUN apk add --no-cache git && \
    go install github.com/air-verse/air@latest

# Copy project files
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build application
RUN CGO_ENABLED=0 GOOS=linux go build -o vrcmemes-bot

# Run application
CMD ["./vrcmemes-bot"] 
