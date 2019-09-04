package dhcpserver

// Directives that we register at caddy
var Directives = []string{
	"interface",
	"option",
	"servername",
	"next-server",
	"lease",
	"range",
}
