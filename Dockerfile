FROM golang:1.18-alpine as builder

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -v -o nomad-pipeline

FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/nomad-pipeline /nomad-pipeline

ENTRYPOINT ["/nomad-pipeline"] 
