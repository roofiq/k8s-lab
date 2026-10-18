FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /worker ./cmd/worker

FROM scratch
COPY --from=build /worker /worker
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/worker"]
