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

The IP addresses are reserved for their clients in the lease database, so other plugins, like [*range*](../ranges), never lease them to other clients, even if they are part of one of their ranges. If another client already has a lease of a static IP address when NextDHCP starts, the lease is removed from the database: the client keeps using the address until it tries to renew the lease, which is refused, and then gets another address.

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