---
title: "interface"
date: 2019-09-20T19:00:00+02:00
draft: false
---


# interface

## Name

*interface* - allows to configure the interface a subnet listener should bind to

## Description

By default NextDHCP tries to find the interface that has the IP address of the subnet to serve assigned.
As this may fail for various reasons the *interface* plugin can be used to tell NextDHCP which interface
should be used. This plugin/directive should **not** be use more than once.

## Syntax

```
interface IFNAME
```

* **IFNAME** is the name of the network interface as reported by `ifconfig` or `ip addr`. NextDHCP will error out if the interface does not exist

## Examples

The most basic example is to tell NextDHCP that it should serve a given subnet (10.1.0.1/8) on `eth0`:

```
10.1.0.1/8 {
    interface eth0
}
```

Working with VLANs can also be implemented by using the `interface` directive:
(assuming *eth0.300* is a tagged VLAN port)

```
10.0.300.0/24 {
    interface eth0.300
}
```

