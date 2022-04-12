# Build Stage
FROM golang:1.16

ENV GOPATH /go
ENV GOBIN /go/bin
WORKDIR /go/src/github.com/nextdhcp

COPY . .
RUN make build

# Release Stage
FROM busybox:glibc

WORKDIR /app
COPY --from=0 /go/src/github.com/nextdhcp/build/nextdhcp /app/nextdhcp
COPY ./Dhcpfile /app/

ENTRYPOINT ["/app/nextdhcp"]