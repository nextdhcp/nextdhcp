plugin "etcd-database" {
    path = "path/to/plugin.so",
    endpoint = "http://localhost:4001/"
}

subnet "10.100.0.1/24" {
    -- database = "etcd-database",

    ranges = {
        {"10.100.0.100", "10.100.0.200"},
    },

--[[
    options = {
        dns = {"8.8.8.8", "1.1.1.1"},
        routers = {"10.100.0.1"},
    },
]]--

    leaseTime = "10m",
    -- leaseTime = 600

    -- offer is called for each DHCPDISCOVER message
    -- the handler will fill in all information that has not been set by the lua handler
    -- already. Thus, if no offer handler is defined IP addresses will be leased based on
    -- configured ranges
    offer = function(ctx, req)
        print(ctx.Resp:Summary())
        ctx.SkipRequest(ctx)
    end,
}
