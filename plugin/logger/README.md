---
title: "log"
date: 2019-11-15T18:10:15+02:00
draft: false
---

# log

## Name

*log* - configure NextDHCPs logging output

## Description

Using the *log* directive it is possible to configure the log level of NextDHCP.

## Syntax

```
log LEVEL
```

* **LEVEL** is the log level to use. Valid levels are `debug`, `info`, `warn` and `error`. Default is `info`.

## Examples

To enable debug logging place the following line in your Dhcpfile:

```
10.1.0.100 - 10.1.0.150 {
    #
    # ...
    #

    log debug
}
```
