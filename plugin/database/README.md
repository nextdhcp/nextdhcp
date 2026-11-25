---
title: "database"
date: 2019-09-20T19:00:00+02:00
draft: false
---

# database

## Name

*database* - configures the lease database to use

## Description

The *database* plugin configures the lease database to use. Most users will likely don't need to use it as the default [bbolt](https://github.com/etcd-io/bbolt) driver will be used. Note that the database plugin only allows configuration of
databases that have been compiled into NextDHCP.

## Syntax

```
database NAME OPTION...
```

* **NAME** is the name of the database driver to use
* **OPTION** is one or more options for the database driver. Please refer to the
documentation of the driver you like to use for more information.

The *database* plugin also allows a block based key-value configuration that has
the following syntax:

```
database NAME {
    KEY VALUE
    ...
}
```

**NAME** is the name of the database driver and **KEY**/**VALUE** specify driver related configuration parameters.

## Examples

The built-in *bolt* driver stores leases in a [bbolt](https://github.com/etcd-io/bbolt) database file. The path of the file is either the option after the driver name, or the value of the `file` key:

```
192.168.0.1/24 {
    database bolt ./leases.db
}
```

The *bolt* driver also has the following keys:

* `timeout` is how long to wait for the database file if another process, like another NextDHCP, has it open, before NextDHCP fails to start. Defaults to `5s`; `0` waits forever. Two server blocks can't use the same database file.
* `mode` is the permissions, in octal, of the database file if NextDHCP creates it. Defaults to `0660`.

```
192.168.0.1/24 {
    database bolt {
        file ./leases.db
        timeout 10s
        mode 0600
    }
}
```

There's also the built-in *memory* database driver. Note that this driver does not
persist leases in any way so a restart of NextDHCP will discard all active leases.

**Use with care**

```
192.168.0.1/24 {
    database memory
}
```