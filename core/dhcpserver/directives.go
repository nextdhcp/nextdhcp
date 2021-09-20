package dhcpserver

// Directives that we register at caddy
var Directives = []string{
	"prometheus",
	"log",
	"database",
	"interface",
	"gotify",
	"mqtt",
	"option",
	"servername",
	"next-server",
	"bootfile",
	"lease",
	"static",
	"range",
}
