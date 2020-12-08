package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sp2pfs/common"

	"github.com/google/uuid"
)

var connections []net.Conn

type DescriptorHeader struct {
	DescriptorID      [16]byte
	PayloadDescriptor uint8
	TTL               uint8
	Hops              uint8
	PayloadLength     uint32
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

func Ping() {
	uuid := uuid.New()
	pingMessage := CreatePingMessage(uuid)

	for _, conn := range connections {
		conn.Write(pingMessage)
	}
}

func Pong() {

}

func ListenForConnections() {
	l, err := net.Listen("tcp", ":"+os.Getenv("SERVENT_PORT"))
	if err != nil {
	}
	for {
		_, err := l.Accept()
		if err != nil {
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
		fmt.Println("First servent")
		successfullyConnected = true
	}

	fmt.Println(serventList)

	for _, servent := range serventList {
		_, err := net.Dial("tcp", servent.Host+":"+servent.Port)
		if err != nil {
			fmt.Println("Error connecting to servent: ", err.Error())
		} else {
			fmt.Println("Succesfully connected to: ", servent.Host+":"+servent.Port)
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

	l, err := net.Listen("tcp", ":"+os.Getenv("SERVENT_PORT"))
	if err != nil {
	}
	for {
		_, err := l.Accept()
		if err != nil {
		}
	}
}
