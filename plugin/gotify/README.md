# gotify

## Name

*gotify* - Send notifications via [Gotify](https://gotify.net)

## Description

The *gotify* plugin can send push notifications about IP addresses and client requests. In order to
authenticate against the gotify server please make sure to create a new application token first.

## Syntax

The *gotify* plugin supports two different configuration modes.

```
gotify [CONDITION] {
    [server SERVER TOKEN]
    [message MESSAGE]
}

gotify [CONDITION]
```

