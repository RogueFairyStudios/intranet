FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/studio-server ./cmd/studio-server

# distroless/static ships CA certificates required for Tellurian Authentik OIDC discovery/JWKS and PostgreSQL TLS.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/studio-server /studio-server
ENTRYPOINT ["/studio-server"]
