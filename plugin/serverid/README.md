---
title: "serverid"
date: 2019-09-20T19:00:00+02:00
draft: false
---

# serverid

## Name

*serverid* - Configures a server identifier and drop requests for other servers

(TBD: add RFC)

## Description

The *serverid* plugin configures a DHCP server identifier that is appended to DHCPOFFER messages. All client messages that have a
server identifier specified that does not match the configured one are dropped.

## Syntax

```
serverid
```

## Examples

```
10.1.0.1/24 {
    serverid
    
    range 10.1.0.100 10.1.0.200
    option router 10.1.0.1
}
```
