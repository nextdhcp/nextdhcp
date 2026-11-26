---
title: "static"
date: 2019-09-20T19:00:00+02:00
draft: false
---

# static

## Name

*static* - MAC address based static IP addresses

## Description

The *static* plugin allows configuration of static IP address based on the MAC address of the requesting client.

The IP addresses are reserved for their clients in the lease database, so other plugins, like [*range*](../ranges), never lease them to other clients, even if they are part of one of their ranges. If the lease database holds another client's lease of a static IP address that hasn't expired, for example from *range* before the address was made static, or because the address was assigned to another client in the configuration, the address isn't given to its client while that lease lasts, so that two clients don't use it at the same time. The lease ends when the other client asks for the address again, for example to renew its lease, which it is refused, or when the lease expires. So if the other client doesn't ask, for example a device that was replaced by another one, the client it's assigned to now waits until that lease expires, which can take as long as the lease time. Until then, the client is served like any other, for example by *range*, which may give it another address, and it moves to its static address when it next renews. The static plugin stores the lease of a static IP address each time its client requests it, like other leases. So if the address is removed from the configuration, it stays leased to the client until that lease expires: the client can renew it if it is part of a range, and gets another address otherwise.

## Syntax

```
static MAC IP
```
where

* **MAC** is the MAC address of the client (like "aa:bb:cc:dd:ee:ff") and
* **IP** is the IP address that should be assigned (like "192.168.0.10")

## Examples

```
10.1.0.1/24 {
    lease 1h
    static 00:aa:de:ad:be:ef 10.1.0.10
    range 10.1.0.100 10.1.0.200
}
```

A static IP address may also be part of a range. Here, other clients get the addresses from 10.1.0.11 to 10.1.0.20:

```
10.1.0.1/24 {
    range 10.1.0.10 10.1.0.20
    static 00:aa:de:ad:be:ef 10.1.0.10
}
```