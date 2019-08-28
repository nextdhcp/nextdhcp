package lua

import (
	"log"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/ppacher/dhcp-ng/pkg/middleware"
	lua "github.com/yuin/gopher-lua"
	luar "layeh.com/gopher-luar"
)

// luaMiddlware is a middleware.Handler that synchronizes execution of DHCP
// requests on the same lua runner
type luaMiddlware struct {
	fn     *lua.LFunction
	runner *Runner
}

func (m *luaMiddlware) Serve(ctx *middleware.Context, req *dhcpv4.DHCPv4) {
	m.runner.loop.ScheduleAndWait(func(L *lua.LState) {
		L.Push(m.fn)
		L.Push(luar.New(L, ctx))
		L.Push(luar.New(L, req))

		if err := L.PCall(2, 0, nil); err != nil {
			// TODO(ppacher) better logging
			log.Println("failed to execute lua handler: ", err.Error())
			ctx.SkipRequest()
		}
	})
}
