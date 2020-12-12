package common

import (
	"net"
	"strings"
)

type Address struct {
	Host string
	Port string
}

func GetIPFromConnection(conn net.Conn) string {
	return strings.Split(conn.RemoteAddr().String(), ":")[0]
}
