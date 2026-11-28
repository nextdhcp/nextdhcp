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

NextDHCP logs to standard error with [zerolog](https://github.com/rs/zerolog). The log level applies to all logs, and logs are human-readable if standard error is a terminal and JSON otherwise.

Each log message has the fields `level`, `time` and `message`. Most also have the field `logger`, which names the part of NextDHCP that logged the message, like `dhcp.handlers.range` for the *range* plugin. Messages about a DHCP request have the fields `hwaddr`, `xid`, `secs` and `msgtype` of the request, and `hostname` if the client sent its host name.

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
