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
    [message MESSAGE]
}
```

## Examples

This example will send a notification for each client that requests a new IP address:

```
gotify msgtype == 'REQUEST' and clientip != '' {
    server https://gotify.example.com Adk57Vkdh487
    message 'Client {hwaddr} ' 
}
```