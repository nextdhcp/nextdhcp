ARG BUILDPLATFORM
FROM ${BUILDPLATFORM}golang:1.16 as build

ENV GOPATH /go
ENV GOBIN /go/bin
WORKDIR /go/src/github.com/nextdhcp

COPY . .
RUN make build

ARG BUILDPLATFORM
FROM ${BUILDPLATFORM}busybox:glibc as release

WORKDIR /app
COPY --from=build /go/src/github.com/nextdhcp/build/nextdhcp /app/nextdhcp
COPY ./Dhcpfile /app/

ENTRYPOINT [ "/app/nextdhcp" ]