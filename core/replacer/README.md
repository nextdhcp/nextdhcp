# Replacer

Replacer allows to access certain DHCP message properties.
It may be used to construct log messages, message bodies for
notifications or may be used for conditions. 

## Syntax

The package is capable of replacing pre-defined keys as well as
custom keys provided by various plugins. If operating on a text
string the replacer expects keys to be encapsulated in `{}`. For example,
`{clientip}` will be replaced with the IP address of the client.

## Predefined keys

The following table lists all replacement keys that are supported by
default:

| KEY         | EXAMPLE              | DESCRIPTION                         |
|-------------|----------------------|-------------------------------------|
| msgtype     | "DISCOVER", "REQUEST"| The message type                    |
| yourip      | "10.0.0.1"           | The IP address of the yiaddr field  |
| clientip    | "192.168.0.100"      | The current IP address of the client|
| hwaddr      | "de:ad:be:ef:01:02"  | The MAC address of the client       |
| requestedip | "192.168.0.101"      | The IP requested by the client      |
| hostname    | "example.com"        | The hostname of the client          |
| gwip        | "10.17.0.2"          | The IP address of the relay host    |
| state       | "renew", "binding"   | The current state of the client     |

## Example

The template

```
Client {hostname} ({hwaddr}) requested {requestedip}
```

will result in 

```
Client ubundu-lab-1 (de:ad:be:ef:01:02) requested 10.1.0.2
```
