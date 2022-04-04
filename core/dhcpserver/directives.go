package dhcpserver

// Directives that we register at caddy
var Directives = []string{
	"logger",
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
	"prometheus",
}
