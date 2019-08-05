home = subnet("10.8.254.0/32", {
    range = {"10.8.1.100", "10.8.1.199"},
    lease_time = 300,
    options = {
        routers = {"10.8.1.254"},
        broadcast_address = "10.8.1.255",
        domain_names = {"paz.dobnet.lan.", "dobnet.lan."},
        domain_name_servers = {"10.89.1.254"}
    }
})

declare_option("architecture", 93, TYPE_UINT16)

return function(request)
    local opts = {}

    if request.architecture == 0x0007 then
        opts.filename = "/grub/x86_64-efi/core.fi"
    else
        opts.filename = "/grub/i386-pc/core.0"
    end
    
    return assign(home, opts)
    --[[

    return {
        address =  "10.8.1.100"
        options = {
            ...
        }
    }

    ]]
end