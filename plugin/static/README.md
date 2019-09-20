---
title: "static"
date: 2019-09-20T19:00:00+02:00
draft: false
---

# static

## Name

*static* - MAC address based static IP addresses

## Description

The *static* plugin allows configuration of static IP address based on the MAC address of the requesting client

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
    leaseTime 1h
    static 00:aa:de:ad:be:ef 10.1.0.10
    range 10.1.0.100 10.1.0.200
}
```