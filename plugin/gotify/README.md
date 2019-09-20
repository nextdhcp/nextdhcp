---
title: "gotify"
date: 2019-09-20T19:00:00+02:00
draft: false
---

# gotify

## Name

*gotify* - Send notifications via [Gotify](https://gotify.net)

## Description

The *gotify* plugin can send push notifications about IP addresses and client requests. In order to
authenticate against the gotify server please make sure to create a new application token first. If a
gotify directive does not specify a server address and token it will use them from a previously defined
directive. If not server address and token can be found an error is thrown. In other words, the first
gotify directive MUST have a server and token configured. It is also possible to omit the message and
condition to only declare the authentication token and server address.

## Syntax

```
gotify [CONDITION] {
    [server SERVER TOKEN]
    [title TITLE]
    [message MESSAGE]
}
```

* **CONDITION** is the condition that must match to send a notification via gotify. If the condition is omitted, a notification
will be sent for each message.  
* **SERVER** is the server address of where gotify is running  
* **TOKEN** is the application token generated on the gotify server
* **TITLE** is the title of the notification. All [replacement keys](../../core/replacer/README.md) are supported.
* **MESSAGE** is the message that should be sent. All [replacement keys](../../core/replacer/README.md) are supported.

## Examples

This example will send a notification for each client that requests a new IP address:

```
gotify msgtype == 'REQUEST' && clientip != '' {
    server https://gotify.example.com Adk57Vkdh487
    title '{msgtype} by {hostname}'
    message 'Client {hwaddr} ' 
}
```