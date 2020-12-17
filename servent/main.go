package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sp2pfs/common"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

const PING_INTERVAL = 10
const PING_DESCRIPTOR = 0x00
const PONG_DESCRIPTOR = 0x01

type ConcurrentServentListMap struct {
	data map[string]struct{}
	sync.RWMutex
}

func NewConcurrentServentListMap() *ConcurrentServentListMap {
	var concurrentServentListMap ConcurrentServentListMap
	concurrentServentListMap.data = make(map[string]struct{})

	return &concurrentServentListMap
}

func (m *ConcurrentServentListMap) Add(key string) {
	m.Lock()
	defer m.Unlock()

	m.data[key] = struct{}{}
}

func (m *ConcurrentServentListMap) HasKey(key string) bool {
	m.Lock()
	defer m.Unlock()

	_, keyFound := m.data[key]

	return keyFound
}

var myAddress string = GetMyAddress().String()

var connections map[string]net.Conn = make(map[string]net.Conn)

var messageIDCache *ConcurrentServentListMap = NewConcurrentServentListMap()

type DescriptorHeader struct {
	DescriptorID      [16]byte
	PayloadDescriptor uint8
	TTL               uint8
	Hops              uint8
	PayloadLength     uint32
}

type PongMessage struct {
	Port                    uint16
	IPAddress               [4]byte
	NumberOfSharedFiles     uint32
	NumberOfKilobytesShared uint32
}

func GetMyAddress() net.IP {
	interfaces, err := net.Interfaces()
	if err != nil {
		fmt.Println("Error getting interfaces: ", err.Error())
	}

	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			fmt.Println("Error getting addreses from interface: ", err.Error())
			return nil
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

			return ip
		}
	}

	return nil
}

func CreateDescriptorHeader(descriptorId [16]byte, payloadDescriptor, ttl, hops uint8, payloadLength uint32) []byte {
	descriptorHeader := DescriptorHeader{
		DescriptorID:      descriptorId,
		PayloadDescriptor: payloadDescriptor,
		TTL:               ttl,
		Hops:              hops,
		PayloadLength:     payloadLength,
	}

	var descriptorHeaderBuffer bytes.Buffer
	err := binary.Write(&descriptorHeaderBuffer, binary.LittleEndian, descriptorHeader)

	if err != nil {
		fmt.Println("Error writing to descriptor header buffer: ", err.Error())
		return nil
	}

	return descriptorHeaderBuffer.Bytes()
}

func CreatePingMessage() []byte {
	descriptorID := uuid.New()
	descriptorData := CreateDescriptorHeader(descriptorID, PING_DESCRIPTOR, 7, 0, 0)

	messageIDCache.Add(string(descriptorID[:]))

	return descriptorData
}

func CreatePongMessage(ttl uint8) []byte {
	descriptorID := uuid.New()
	descriptorHeader := CreateDescriptorHeader(descriptorID, PONG_DESCRIPTOR, ttl, 0, 14) //payload size hardcoded
	port, err := strconv.ParseUint(os.Getenv("SERVENT_PORT"), 10, 16)
	if err != nil {
		fmt.Println("Error converting port to int64: ", err.Error())
	}

	ipAddr := GetMyAddress()

	pongMessage := PongMessage{
		Port:                    uint16(port),
		IPAddress:               [4]byte{ipAddr[3], ipAddr[2], ipAddr[1], ipAddr[0]},
		NumberOfSharedFiles:     0,
		NumberOfKilobytesShared: 0,
	}

	var pongMessageBuffer bytes.Buffer
	err = binary.Write(&pongMessageBuffer, binary.LittleEndian, pongMessage)
	if err != nil {
		fmt.Println("Error writing to the pong message buffer: ", err.Error())
		return nil
	}

	return append(descriptorHeader, pongMessageBuffer.Bytes()...)
}

func SendPing() {
	pingMessage := CreatePingMessage()

	for _, conn := range connections {
		conn.Write(pingMessage)
	}
}

func SendMessageToAllConnectionsExcept(message []byte, exclusionMap map[string]struct{}) {
	fmt.Println(myAddress, "connections: ", connections)
	for key, conn := range connections {
		if _, ok := exclusionMap[key]; ok == false {
			fmt.Println(myAddress, " writing ", message, " to ", common.GetIPFromConnection(conn))
			conn.Write(message)
		}
	}
}

func SendPong(conn net.Conn, ttl uint8) {

}

func SendPingPeriodically() {
	ticker := time.NewTicker(PING_INTERVAL * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fmt.Println(myAddress, " - ", connections)
			SendPing()
		}
	}
}

func AddConnection(ip string, conn net.Conn) {
	if _, found := connections[ip]; found == false {
		fmt.Println(myAddress, "adding connection: ", conn.RemoteAddr())
		connections[ip] = conn
	} else {
		fmt.Println(myAddress, "connection already open: ", ip)
	}
}

func AddConnectionAndStartWaitingForMessages(ip string, conn net.Conn) {
	if _, found := connections[ip]; found == false {
		fmt.Println(myAddress, "adding connection: ", conn.RemoteAddr())
		connections[ip] = conn
		go WaitForMessagesFromConnection(conn)
	} else {
		fmt.Println(myAddress, "connection already open: ", ip)
	}
}

func ProcessPingMessage(conn net.Conn, descriptorHeader DescriptorHeader) {
	msgForForwarding := new(bytes.Buffer)
	err := binary.Write(msgForForwarding, binary.LittleEndian, descriptorHeader)
	if err != nil {
		fmt.Println("Error writing the message for forwarding: ", err.Error())
	}

	SendMessageToAllConnectionsExcept(
		msgForForwarding.Bytes(), map[string]struct{}{common.GetIPFromConnection(conn): {}})
}

func ProcessPongMessage(conn net.Conn, descriptorHeader DescriptorHeader, pongMessage PongMessage) {
	fmt.Println("Pong message received: ", pongMessage)
}

func ProcessMessage(conn net.Conn, message []byte) {
	var descriptorHeader DescriptorHeader
	msgBuffer := bytes.NewBuffer(message)
	err := binary.Read(msgBuffer, binary.LittleEndian, &descriptorHeader)
	if err != nil {
		fmt.Println("Error reading descriptor header: ", err.Error())
	}

	descriptorID := string(descriptorHeader.DescriptorID[:])

	if messageIDCache.HasKey(descriptorID) {
		fmt.Println("Mesage already processed")
		return
	}

	messageIDCache.Add(descriptorID)

	descriptorHeader.Hops++
	descriptorHeader.TTL--

	if descriptorHeader.TTL == 0 {
		fmt.Println("Message expired")
		return
	}

	switch descriptorHeader.PayloadDescriptor {

	case PING_DESCRIPTOR:
		ProcessPingMessage(conn, descriptorHeader)

	case PONG_DESCRIPTOR:
		var pongMessage PongMessage
		err = binary.Read(msgBuffer, binary.LittleEndian, &pongMessage)
		if err != nil {
			fmt.Println("Error reading pong message: ", err.Error())
		}
		ProcessPongMessage(conn, descriptorHeader, pongMessage)
	}
}

func WaitForMessagesFromConnection(conn net.Conn) {
	for {
		message := make([]byte, 1024)
		numRead, err := conn.Read(message)
		message = message[:numRead]
		if err != nil {
			fmt.Println("Error receiving message from connection ", common.GetIPFromConnection(conn), ": ", err.Error())
		}
		fmt.Println(myAddress, " - message received: ", message, "numRead: ", numRead)
		ProcessMessage(conn, message)
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
			AddConnectionAndStartWaitingForMessages(common.GetIPFromConnection(conn), conn)
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
			AddConnectionAndStartWaitingForMessages(servent.Host, conn)
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

	WaitForConnections()
}
