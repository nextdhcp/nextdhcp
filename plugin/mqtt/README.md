---
title: "mqtt"
date: 2019-11-17T12:24:10+02:00
draft: false
---

# mqtt

## Name

*mqtt* - Publish messages to an MQTT enabled server

## Description

The *mqtt* plugin can publish messages about IP addresses and client requests to an MQTT server.
Multiple configuration are supported and may refer to other ones for connection information. See the
examples below for more information.

## Syntax

```
mqtt [CONDITION] {
    [name/use CONFIG_NAME]
    [broker BROKER...]
    [user USERNAME]
    [password PASSWORD]
    [clean-session]
    [qos QOS]
    
    [topic TOPIC]
    [payload PAYLOAD]
}
```

* **CONFIG_NAME** is the name of the MQTT configuration. If specified with *use* the connection information of a MQTT block with the same *name* will be used.
* **CONDITION** is the condition that must match to publish to MQTT. If the condition is omitted, a message
will be published for each DHCP message.  
* **BROKER** is or more MQTT server URLs in the format of `scheme://address:port`. Supported protocols are `tcp://`, `tls://`, `ws://` and `wss://`
* **USER** optionally configures the username required for authentication
* **PASSWORD** optionally configures the username required for authentication
* `clean-session` forces a clean-session for the client
* **QOS** specifies the Quality-of-Service byte to use when publishing. It should be a number between 0 and 2
* **TOPIC** is the topic use to publish the message. All [replacement keys](../../core/replacer/README.md) are supported. A block without a topic doesn't publish anything: it can only configure a connection with a *name* for other blocks to *use*, and must not have a condition or payload.
* **PAYLOAD** is the the actual MQTT message payload. All [replacement keys](../../core/replacer/README.md) are supported. If it is omitted, an empty payload is published.

## Examples

This example will publish two notifications for each client that requests a new IP address to different topics:

```
mqtt msgtype == 'REQUEST' && clientip != '' {
    name default
    broker tcp://localhost:1883
    user mqtt-user-name
    password very-secure-password
    
    topic /dhcp/hwaddr/{hwaddr}
    payload {clientip}
}

# The following block uses escaping to construct a JSON message
# and uses the same connection information from above
mqtt msgtype == 'REQUEST' {
    use default

    topic /dhcp/requested-ip/{clientip}
    payload "\{\"clientIP\": \"{clientip}\"\}"
}
```

A block can also configure only a connection, which other blocks *use* to publish messages:

```
mqtt {
    name local
    broker tcp://localhost:1883
}

mqtt msgtype == 'DISCOVER' {
    use local
    topic /dhcp/discover/{hwaddr}
}
```