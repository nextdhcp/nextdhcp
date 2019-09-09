package dhcpserver

// Directives that we register at caddy
var Directives = []string{
	"database",
	"interface",
	"option",
	"servername",
	"next-server",
	"lease",
	"static",
	"range",
}
