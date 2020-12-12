package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sp2pfs/common"
	"time"

	"github.com/google/uuid"
)

const PING_INTERVAL = 10

var myAddress string = GetMyAddress()

var connections map[string]net.Conn = make(map[string]net.Conn)

type DescriptorHeader struct {
	DescriptorID      [16]byte
	PayloadDescriptor uint8
	TTL               uint8
	Hops              uint8
	PayloadLength     uint32
}

func GetMyAddress() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		fmt.Println("Error getting interfaces: ", err.Error())
	}

	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			fmt.Println("Error getting addreses from interface: ", err.Error())
			return ""
		}

		for _, addr := range addrs {
			var ip net.IP
			switch t := addr.(type) {
			case *net.IPNet:
				ip = t.IP
			case *net.IPAddr:
				ip = t.IP
			}

			if ip == nil || ip.IsLoopback() {
				continue
			}

			ip = ip.To4()
			if ip == nil {
				continue
			}
			return ip.String()
		}
	}

	return ""
}

func CreateDescriptorHeader(descriptorId [16]byte, payloadDescriptor, ttl, hops uint8, payloadLength uint32) []byte {
	data, err := json.Marshal(DescriptorHeader{
		DescriptorID:      descriptorId,
		PayloadDescriptor: payloadDescriptor,
		TTL:               ttl,
		Hops:              hops,
		PayloadLength:     payloadLength,
	})
	if err != nil {
		fmt.Println("Error marshalling header descriptor: ", err.Error())
		return nil
	}

	return data
}

func CreatePingMessage(descriptorId [16]byte) []byte {
	descriptorData := CreateDescriptorHeader(descriptorId, 0x01, 7, 0, 0)

	return descriptorData
}

func SendPing() {
	uuid := uuid.New()
	pingMessage := CreatePingMessage(uuid)

	for _, conn := range connections {
		conn.Write(pingMessage)
	}
}

func SendPingPeriodically() {
	ticker := time.NewTicker(PING_INTERVAL * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			SendPing()
		}
	}
}

func Pong() {

}

func AddConnection(ip string, conn net.Conn) {
	if _, found := connections[ip]; found == false {
		connections[ip] = conn
	} else {
		fmt.Println(myAddress, "connection already open: ", ip)
	}
}

func WaitForMessages() {
	for _, conn := range connections {
		go func(c net.Conn) {
			for {
				message := make([]byte, 1024)
				_, err := c.Read(message)
				if err != nil {
					fmt.Println("Error receiving message from connection: ", err.Error())
				}
				fmt.Println(myAddress, " - received message - ", string(message))
			}
		}(conn)
	}
}

func WaitForConnections() {
	listener, err := net.Listen("tcp", ":"+os.Getenv("SERVENT_PORT"))

	if err != nil {
		fmt.Println("Error creating servent listener:", err.Error())
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting new connection: ", err.Error())
		} else {
			AddConnection(common.GetIPFromConnection(conn), conn)
			fmt.Println(myAddress, " - accepted connection ", conn.RemoteAddr())
		}
	}
}

func main() {
	connectServiceConn, err := net.Dial("tcp", "connect_service:8080")

	if err != nil {
		fmt.Println("Error dialing connect service: ", err.Error())
		return
	}
	defer connectServiceConn.Close()

	successfullyConnected := false

	serventListData := make([]byte, 1024)
	numRead, err := connectServiceConn.Read(serventListData)
	if err != nil {
		fmt.Println("Error reading servent list from connect service: ", err.Error())
		return
	}

	serventListData = append([]byte(nil), serventListData[:numRead]...)
	var serventList []common.Address
	err = json.Unmarshal(serventListData, &serventList)
	if err != nil {
		fmt.Println("Error unmarshalling servent list: ", err.Error())
		return
	}

	if len(serventList) == 0 {
		fmt.Println(myAddress, " - first servent")
		successfullyConnected = true
	}

	fmt.Println(myAddress, " - ", serventList)

	for _, servent := range serventList {
		conn, err := net.Dial("tcp", servent.Host+":"+servent.Port)
		if err != nil {
			fmt.Println("Error connecting to servent: ", err.Error())
		} else {
			AddConnection(servent.Host, conn)
			fmt.Println(myAddress, " - succesfully connected to: ", servent.Host+":"+servent.Port)
			successfullyConnected = true
		}
	}

	var port string = "0"
	if successfullyConnected {
		port = os.Getenv("SERVENT_PORT")
	}

	portData, err := json.Marshal(port)
	if err != nil {
		fmt.Println("Error marshalling port number: ", err.Error())
	}

	_, err = connectServiceConn.Write(portData)
	if err != nil {
		fmt.Println("Error sending port to connect service: ", err.Error())
	}

	go SendPingPeriodically()
	go WaitForMessages()

	WaitForConnections()
}
