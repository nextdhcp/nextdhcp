---
title: "range"
date: 2019-09-20T19:00:00+02:00
draft: false
---

# range

## Name

*range* - configure a range of IP address to be leased

## Description

The *range* plugin allows to dynamically lease IP addresses to requesting clients. Each client requesting 
will be assigned an IP address from one of the configured ranges. If the client has already been assigned
an address (i.e. in RENEWING state) it will get the very same address assigned. If the `range` plugin is
not able to find a suitable address the next plugin will be called. Typically, the `range` plugin should
be one of the last plugins used. This plugin may be specified multiple times per DHCP server block.

## Syntax

```
range START_IP END_IP
```

* **START_IP** is the (inclusive) start IP of the range (like `192.168.0.1`)
* **END_IP** is the (inclusive) end IP of the range (like `192.168.0.100`)

## Examples

The following example allows dynamic clients to receive IP addresses from two different pools:

```
192.168.0.1/24 {
    range 192.168.0.100 192.168.0.150
    range 192.168.0.200 192.168.0.250
}
```