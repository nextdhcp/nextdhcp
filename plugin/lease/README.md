---
title: "lease"
date: 2019-09-20T19:00:00+02:00
draft: false
---

# lease

## Name

*lease* - configure the allowed lease time for an IP address

## Description

The *lease* plugin allows to configure the valid life-time of an IP address lease. It only sets the IP address lease time if no other plugin set it. The *lease* option SHOULD be used in almost all NextDHCP setups.

## Syntax

```
lease DURATION
```

* **DURATION** is the duration for which a lease is valid. The format should follow the [time.Duration](https://godoc.org/golang.org/time) format supported by [Go](https://golang.org)

## Examples

The *lease* directive supports all duration formats that Go supports. Thus, the following directive are all valid:

```
lease 1h
lease 1d3m
lease 3m30s
lease 1y10m3s
```

An example could look like:

```
192.168.0.1/24 {
    lease 1d
}
```