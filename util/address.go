package util

import (
	"net"
	"strconv"
	"strings"
)

type Address struct {
	Host string
	Port string
}

func GetIPFromConnection(conn net.Conn) string {
	return strings.Split(conn.RemoteAddr().String(), ":")[0]
}

func ConvertBytesToIpAddress(ipAddressBytes [4]byte) string {
	return strconv.FormatUint(uint64(ipAddressBytes[0]), 10) + "." +
		strconv.FormatUint(uint64(ipAddressBytes[1]), 10) + "." +
		strconv.FormatUint(uint64(ipAddressBytes[2]), 10) + "." +
		strconv.FormatUint(uint64(ipAddressBytes[3]), 10)

}
