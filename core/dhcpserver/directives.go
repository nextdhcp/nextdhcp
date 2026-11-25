package dhcpserver

// Directives lists the directives of the Dhcpfile. Directives are set up in
// this order, and handlers are added to the chain in this order.
var Directives = []string{
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
