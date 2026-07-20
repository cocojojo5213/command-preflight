FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/command-preflight-server ./cmd/command-preflight-server
RUN mkdir -p /image-data && chmod 0770 /image-data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/command-preflight-server /command-preflight-server
COPY --from=build --chown=65532:65532 /image-data /data
VOLUME ["/data"]
EXPOSE 8787
ENTRYPOINT ["/command-preflight-server"]
