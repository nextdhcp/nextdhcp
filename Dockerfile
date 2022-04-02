FROM golang:1.18.0 AS go-builder

COPY ./ /go/src/github.com/nextdhcp
ENV GOPATH /go
ENV GOBIN /go/bin
WORKDIR /go/src/github.com/nextdhcp
RUN make build

FROM busybox:glibc 

WORKDIR /app
COPY --from=go-builder /go/src/github.com/nextdhcp/build/nextdhcp /app/nextdhcp
COPY ./Dhcpfile /app/

ENTRYPOINT [ "/app/nextdhcp" ]