FROM golang:1.26-alpine AS build
RUN apk add --no-cache ca-certificates
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /worker ./cmd/worker

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /worker /worker
USER 65532:65532
ENTRYPOINT ["/worker"]
