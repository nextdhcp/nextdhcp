package dhcpserver

// Directives that we register at caddy
var Directives = []string{
	"log",
	"database",
	"interface",
	"serverid",
	"gotify",
	"option",
	"servername",
	"next-server",
	"lease",
	"static",
	"range",
}
