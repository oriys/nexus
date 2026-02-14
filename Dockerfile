FROM golang:1.24-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /nexus ./cmd/nexus

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /nexus /nexus
COPY configs/nexus.yaml /etc/nexus/nexus.yaml

EXPOSE 8080 8443 9090

ENV NEXUS_CONFIG=/etc/nexus/nexus.yaml

ENTRYPOINT ["/nexus"]
