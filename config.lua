plugin "etcd-database" {
    path = "path/to/plugin.so",
    endpoint = "http://localhost:4001/"
}

subnet "10.1.0.1/24" {
    database = "etcd-database",
    
    ranges = {
        {"10.1.0.100", "10.1.0.200"},
    }

    options = {
        dns = {"10.1.0.1", "8.8.8.8", "1.1.1.1"},
        routers = {"10.1.0.1"},
    }
    
    leaseTime = "10m",
    -- leaseTime = 600

    -- offer is called for each DHCPDISCOVER message
    -- the handler will fill in all information that has not been set by the lua handler
    -- already. Thus, if no offer handler is defined IP addresses will be leased based on
    -- configured ranges
    offer = function(ctx, discover, offer)
        -- silently drop DHCPDISCOVER requests from de:ad:be:ef:00:00
        if discover.hardware_address == "de:ad:be:ef:00:00" then
            ctx:drop()
            return
        end

        if discover.hardware_address == "aa:bb:cc:dd:ee:ff" then
            offer.ip = "10.1.0.2"
            offer.options.next_server = "10.1.0.100"
            offer.options.filename = "boot/pxe.0"
        end
    end,
}
