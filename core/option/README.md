# Well-Known options

NextDHCP include the `core/option` package that contains a list of well-known options that can be used in various places like the [replacer](../replacer), [matcher](../matcher) or the [option plugin](../../plugin/option). The following table contains all options that are currently known by NextDHCP.

| Key                    | Option Type          |
|------------------------|----------------------|
| router                 | IP-List              |
| nameserver             | IP-List              |
| ntp-server             | IP-List              |
| server-identifier      | IP-List              |
| broadcast-address      | IP                   |
| requested-ip           | IP                   |
| netmask                | IP                   |
| hostname               | String               |
| domain-name            | String               |
| root-path              | String               |
| class-identifier       | String               |
| tftp-server-name       | String               |
| filename               | String               |
| user-class-information | String-List          |

