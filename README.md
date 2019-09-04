# NextDHCP

A DHCP server that chains middlewares. Similar to Caddy and CoreDNS

[![Build Status](https://travis-ci.com/nextdhcp/nextdhcp.svg?branch=master)](https://travis-ci.com/nextdhcp/nextdhcp)
[![codecov](https://codecov.io/gh/ppacher/dhcp-ng/branch/master/graph/badge.svg)](https://codecov.io/gh/nextdhcp/nextdhcp)
[![Go Report Card](https://goreportcard.com/badge/github.com/nextdhcp/nextdhcp)](https://goreportcard.com/report/github.com/nextdhcp/nextdhcp)


NextDHCP is an easy to use and extensible DHCP server that chains plugins. It's based on the [Caddy server framework](https://github.com/caddyserver/caddy/) and is thus similar to [Caddy](https://caddyserver.com/) and [CoreDNS](https://coredns.io/). 

## Getting Started

The following instructions will get you a local copy of the project for development and testing purposes. For production deployments see "Deployment" below. Note that this project is still in early alpha and may not yet be considered stable.

### Prerequisites

In order to install, hack and test NextDHCP you need a working [Go](https://golang.org) environment. Since this project already adapted go modules you should use at least version 1.12. For testing it is also recommended to have
at least one virtual machine available. 

### Installing

If you just want to install NextDHCP without planning to hack around in it's source code the following command should be enough to install it

```
go get github.com/nextdhcp/nextdhcp
```

This will install the NextDHCP binary into `$GOPATH/bin`. If you want to start hacking on the project follow the steps below:

First clone the repository to a folder of your choice

```
git clone https://github.com/nextdhcp/nextdhcp
```

Finally, enter the directory and build NextDHCP

```
cd nextdhcp
go generate ./...
go build -o nextdhcp ./
```

### Usage

Before starting NextDHCP you need to create a configuration file. See [Dhcpfile](./Dhcpfile) for an example.

```
192.168.0.1/24 {
    lease 1h
    range 192.168.0.100 192.168.0.200
    option router 192.168.0.1
}
```

Next, we need to start the DHCP server as root:

```
sudo ./nextdhcp 
```

## Deployment

There has been no releases of NextDHCP yet so please follow "Getting Started" if you want to setup and test NextDHCP.

## Versioning

We use [SemVer](http://semver.org/) for versioning. For all versions available, see the [tags on this repository](https://github.com/nextdhcp/nextdhcp/tags) or checkout the [release page](https://github.com/nextdhcp/nextdhcp/releases).

## Authors

* **Patrick Pacher** - *Initial work* - [ppacher](https://github.com/ppacher)

See also the list of [contributors](https://github.com/nextdhcp/nextdhcp/graphs/contributors) who participated in this project.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Thank you

This project could not have been built without the following libraries or projects. They are either directly used in NextDHCP or provided a lot of inspiration for the shape of the project:

- [Caddy](https://caddyserver.com)
- [CoreDNS](https://coredns.io)
- [insomniacslk/dhcp](https://github.com/insomniacslk/dhcp)
