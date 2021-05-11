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
	"lease",
	"static",
	"range",
	"prometheus",
}
