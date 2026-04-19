# syntax=docker/dockerfile:1.7

# --- Stage 1: build the Vite frontend ---------------------------------------
FROM node:24-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json* ./
RUN npm ci
COPY web/ ./
RUN npm run build

# --- Stage 2: build the Go binary with the frontend embedded ----------------
FROM golang:1.24-alpine AS gobuild
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Replace the placeholder with the real Vite build so //go:embed picks it up
RUN rm -rf internal/server/dist && mkdir -p internal/server/dist
COPY --from=web /web/dist/ internal/server/dist/

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/home-calendar ./cmd/server

# --- Stage 3: minimal runtime image -----------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=gobuild /out/home-calendar /app/home-calendar
VOLUME ["/data"]
EXPOSE 8080
ENV CONFIG_PATH=/data/config.json LISTEN_ADDR=:8080
USER nonroot:nonroot
ENTRYPOINT ["/app/home-calendar"]
