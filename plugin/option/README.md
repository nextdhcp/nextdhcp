---
title: "option"
date: 2019-09-20T19:00:00+02:00
draft: false
---

# option

## Name

*option* - configures a DHCP option

## Description

The *option* plugin can be used to configure one or more DHCP options for clients
requesting them. It provides some common names for well-known options but can
also be used to configure custom DHCP options. See examples for more information.
The *option* plugin may be used multiple times per server-block.

## Syntax

```
option NAME VALUE...
```

* **NAME** is the well-known name of the option. See below for a list of supported names.
* **VALUE** one or more values for the option. The actual format of the value depends on the option name.

```
option {
    NAME VALUE
    ...
}
```

The *option* plugin allows multiple DHCP options to be configured at once by using a `{}` block add adding multiple NAME/VALUE pairs (one per line).

## Examples

```
10.1.0.1/24 {
    option router 10.1.0.1 10.1.0.2
    option nameserver 10.1.0.1 10.1.0.2
}
```

The next example has the same effect as the above one but this time using
only one *option* directive

```
10.1.0.1/24 {
    option {
        router 10.1.0.1 10.1.0.2
        nameserver 10.1.0.1 10.1.0.2
    }
}
```

## Supported Names

### *router*

Configures default gateways. Expects one or more IP addresses.

```
option router 10.1.0.1 10.1.0.2
```

### *nameserver*

Configures the DNS servers. Expects one or more IP addresses.

```
option nameserver 8.8.8.8 1.1.1.1
```

### *ntp-server*

Configures the time servers to use by clients. Expectes on or more IP
addresses.

```
option ntp-server 1.2.3.4 10.1.1.1
```

### *broadcast-address*

The boardcast address of the local network. Expects an IP address

```
option broadcast-address 192.168.0.255
```

### *netmask*

Configures the netmask of the local network. Expects the mask to be in IP address format.

```
option netmask 255.255.255.0
```

### *hostname*

Configures the hostname that should be used by the client

```
option hostname myhostname
```

### *domain-name*

Configures the domain-name that should be used by the client.

```
option domain-name nextdhcp.io
```

### *tftp-server-name*

This option is used to identify a TFTP server. (Option 66).

```
option tftp-server-name tftp.nextdhcp.io
```

### *tftp-server-addr*

This option sets the TFTP server address (150). Note that [RFC5859](https://tools.ietf.org/html/rfc5859)
defines this option with a higher priority than the *tftp-server-name* (66) option.

### *filename*

The filename that should be loaded during PXE / network boot. (Option 67)

```
option filename pxe.0
```
