---
title: "transport"
date: 2026-11-26T00:00:00+00:00
draft: false
---

# transport

## Name

*transport* - choose how NextDHCP exchanges messages with DHCP clients

## Description

By default, NextDHCP reads DHCP requests from an AF_PACKET socket on the network interface of a subnet, and sends the replies for clients that don't have an IP address yet straight to their hardware address, as RFC 2131 prefers. AF_PACKET sockets need the `CAP_NET_RAW` capability, which containers often don't have.

The *udp* transport uses UDP sockets only, so NextDHCP only needs to be allowed to use port 67, for example with the `CAP_NET_BIND_SERVICE` capability. All subnets share one UDP socket on port 67 of all addresses, and NextDHCP passes each request to the subnet served on the interface the request arrived on. Without access to the link layer, NextDHCP can't send a reply to the hardware address of a client that doesn't have an IP address yet, so it broadcasts those replies instead, which RFC 2131 allows. They reach every host on the network segment, not just the client.

All subnets must use the same transport, or NextDHCP fails to start. With the *udp* transport:

- NextDHCP needs port 67 of all addresses to itself, so it can't share the host's network with another DHCP server or relay agent, even one that serves other interfaces.
- The [static](../static) plugin still uses ARP requests, which need `CAP_NET_RAW`, to check whether a device still uses a static IP address that the lease database has leased to another client. Without the capability, NextDHCP logs a warning and keeps the other client's lease until it ends.

## Syntax

```
transport packet|udp
```

* **packet** reads requests from an AF_PACKET socket, and is the default
* **udp** uses UDP sockets only, and broadcasts the replies that *packet* sends to the hardware address of a client

## Examples

Serve a subnet without the `CAP_NET_RAW` capability, for example in a container:

```
192.168.0.1/24 {
    transport udp
    range 192.168.0.100 192.168.0.200
    lease 1h
}
```

Each server block of a Dhcpfile that serves more than one subnet needs the directive:

```
10.0.0.1/24 {
    transport udp
}

10.0.1.1/24 {
    transport udp
}
```
