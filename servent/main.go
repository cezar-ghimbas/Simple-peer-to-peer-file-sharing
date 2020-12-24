package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"os"
	"sp2pfs/common"
	"sp2pfs/message"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const PING_INTERVAL = 10

var myAddress string = GetMyAddress().String()

var connections map[string]net.Conn = make(map[string]net.Conn) // shared, writable, needs sync

var sharedFileNames []string = CreateFileNames()

var messageCodec message.MessageCodec = new(message.MessageCodecBasic)

var concurrentMessageCache *ConcurrentMessageCache = NewConcurrentMessageCache() // shared, writable, needs sync

type Pair struct {
	values [2]interface{}
}

func CreatePair(value1, value2 interface{}) Pair {
	return Pair{values: [2]interface{}{value1, value2}}
}

type ConcurrentMessageCache struct {
	data map[string]Pair
	sync.RWMutex
}

func NewConcurrentMessageCache() *ConcurrentMessageCache {
	var concurrentMessageCache ConcurrentMessageCache
	concurrentMessageCache.data = make(map[string]Pair)

	return &concurrentMessageCache
}

func (m *ConcurrentMessageCache) Add(key string, value Pair) {
	m.Lock()
	defer m.Unlock()

	m.data[key] = value
}

func (m *ConcurrentMessageCache) AddUnique(key string, value Pair) bool {
	m.Lock()
	defer m.Unlock()

	_, keyFound := m.data[key]
	if keyFound == false {
		m.data[key] = value
	}

	return keyFound
}

func (m *ConcurrentMessageCache) HasKey(key string) bool {
	m.Lock()
	defer m.Unlock()

	_, keyFound := m.data[key]

	return keyFound
}

func GetConnection(concurrentMessageCache *ConcurrentMessageCache, key string) net.Conn {
	concurrentMessageCache.Lock() //Is it necessary?
	defer concurrentMessageCache.Unlock()

	if concurrentMessageCache.data[key].values[0] == nil {
		return nil
	}
	return concurrentMessageCache.data[key].values[0].(net.Conn)
}

func GetDescriptorHeader(concurrentMessageCache *ConcurrentMessageCache, key string) message.DescriptorHeader {
	concurrentMessageCache.Lock() // Is it necessary?
	concurrentMessageCache.Unlock()

	return concurrentMessageCache.data[key].values[1].(message.DescriptorHeader)
}

func PrependDataSize(data []byte) []byte {
	prefix := make([]byte, 4) //////////////////////// hardcoded
	binary.LittleEndian.PutUint32(prefix, uint32(len(data)))
	return append(prefix, data...)
}

func CreateFileNames() []string {
	var resFileNames []string
	rand.Seed(time.Now().UnixNano())
	for i := 0; i < 10; i++ {
		nr := 10 + rand.Intn(20)
		resFileNames = append(resFileNames, fmt.Sprintf("%d", nr))
	}

	return resFileNames
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
	descriptorHeader := message.DescriptorHeader{
		DescriptorID:      descriptorId,
		PayloadDescriptor: payloadDescriptor,
		TTL:               ttl,
		Hops:              hops,
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

	descriptorHeader := message.DescriptorHeader{
		DescriptorID:      descriptorID,
		PayloadDescriptor: message.PING_DESCRIPTOR,
		TTL:               7,
		Hops:              0,
	}

	pingMessageData, err := messageCodec.EncodePingMessage(descriptorHeader)

	if err != nil {
		fmt.Println("Error encoding ping message: ", err.Error())
		return nil
	}

	concurrentMessageCache.Add(string(descriptorID[:]), CreatePair(nil, descriptorHeader)) // synchronization?

	return pingMessageData
}

func CreatePongMessage(descriptorID [16]byte, ttl uint8) []byte {
	port, err := strconv.ParseUint(os.Getenv("SERVENT_PORT"), 10, 16)
	if err != nil {
		fmt.Println("Error converting port to int64: ", err.Error())
	}

	ipAddr := GetMyAddress()

	pongMessageData, err := messageCodec.EncodePongMessage(
		message.DescriptorHeader{
			DescriptorID:      descriptorID,
			PayloadDescriptor: message.PONG_DESCRIPTOR,
			TTL:               ttl,
			Hops:              0,
		},
		message.PongMessage{
			Port:                    uint16(port),
			IPAddress:               [4]byte{ipAddr[0], ipAddr[1], ipAddr[2], ipAddr[3]},
			NumberOfSharedFiles:     0,
			NumberOfKilobytesShared: 0,
		},
	)

	if err != nil {
		fmt.Println("Error encoding pong message: ", err.Error())
		return nil
	}

	return pongMessageData
}

func CreateQueryMessage(minimumSpeed uint16, searchCriteria string) []byte {
	descriptorID := uuid.New()

	descriptorHeader := message.DescriptorHeader{
		DescriptorID:      descriptorID,
		PayloadDescriptor: message.QUERY_DESCRIPTOR,
		TTL:               7,
		Hops:              0,
	}

	queryMessageData, err := messageCodec.EncodeQueryMessage(
		descriptorHeader,
		message.QueryMessage{
			MinimumSpeed:   minimumSpeed,
			SearchCriteria: searchCriteria,
		},
	)
	if err != nil {
		fmt.Println("Error encoding query message: ", err.Error())
	}

	concurrentMessageCache.Add(string(descriptorID[:]), CreatePair(nil, descriptorHeader)) // synchronization?

	return queryMessageData
}

func CreateQueryHitMessage(descriptorID [16]byte, ttl uint8, resultSet []message.QueryResult) []byte {
	port, err := strconv.ParseUint(os.Getenv("SERVENT_PORT"), 10, 16)
	if err != nil {
		fmt.Println("Error converting port to int64: ", err.Error())
	}

	numberOfHits := uint8(len(resultSet))
	ipAddr := GetMyAddress()

	fmt.Println(myAddress, " - data for query hit message creation: ", ttl, resultSet)

	queryHitMessageData, err := messageCodec.EncodeQueryHitMessage(
		message.DescriptorHeader{
			DescriptorID:      descriptorID,
			PayloadDescriptor: message.QUERY_HIT_DESCRIPTOR,
			TTL:               ttl,
			Hops:              0,
		},
		message.QueryHitMessage{
			NumberOfHits: numberOfHits,
			Port:         uint16(port),
			IPAddress:    [4]byte{ipAddr[0], ipAddr[1], ipAddr[2], ipAddr[3]},
			Speed:        0,
			ResultSet:    resultSet,
		},
	)

	if err != nil {
		fmt.Println(myAddress, " - error encoding query hit message: ", err.Error())
		return nil
	}

	return queryHitMessageData
}

func SendNewPingMessage() {
	pingMessage := CreatePingMessage()

	for _, conn := range connections {
		conn.Write(PrependDataSize(pingMessage))
	}
}

func SendMessageToAllConnectionsExcept(message []byte, exclusionMap map[string]struct{}) {
	fmt.Println(myAddress, "connections: ", connections)
	for key, conn := range connections {
		if _, ok := exclusionMap[key]; ok == false {
			fmt.Println(myAddress, " writing ", message, " to ", common.GetIPFromConnection(conn))
			conn.Write(PrependDataSize(message))
		}
	}
}

func SendNewPongMessage(conn net.Conn, descriptorID [16]byte, ttl uint8) {
	pongMessage := CreatePongMessage(descriptorID, ttl)
	conn.Write(PrependDataSize(pongMessage))
}

func SendPingPeriodically() {
	ticker := time.NewTicker(PING_INTERVAL * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fmt.Println(myAddress, " - ", connections)
			SendNewPingMessage()
		}
	}
}

func SendNewQueryMessage(minimumSpeed uint16, searchCriteria string) {
	queryMessageData := CreateQueryMessage(minimumSpeed, searchCriteria)

	for _, conn := range connections {
		conn.Write(PrependDataSize(queryMessageData))
	}
}

func SendNewQueryHitMessage(conn net.Conn, descriptorID [16]byte, ttl uint8, resultSet []message.QueryResult) {
	queryHitMessage := CreateQueryHitMessage(descriptorID, ttl, resultSet)

	conn.Write(PrependDataSize(queryHitMessage))
}

func AddConnection(ip string, conn net.Conn) {
	if _, found := connections[ip]; found == false {
		fmt.Println(myAddress, "adding connection: ", conn.RemoteAddr())
		connections[ip] = conn //synchronize???
	} else {
		fmt.Println(myAddress, "connection already open: ", ip)
	}
}

func AddConnectionAndStartWaitingForMessages(ip string, conn net.Conn) {
	if _, found := connections[ip]; found == false {
		fmt.Println(myAddress, "adding connection: ", conn.RemoteAddr())
		connections[ip] = conn //synchronize???
		go WaitForMessagesFromConnection(conn)
	} else {
		fmt.Println(myAddress, "connection already open: ", ip)
	}
}

func ProcessPingMessage(conn net.Conn, descriptorHeader message.DescriptorHeader) {
	if descriptorHeader.TTL == 0 {
		fmt.Println(myAddress, " - ping message expired")
		return
	}

	descriptorID := string(descriptorHeader.DescriptorID[:])
	messageFound := concurrentMessageCache.AddUnique(descriptorID, CreatePair(conn, descriptorHeader))

	if messageFound &&
		GetDescriptorHeader(concurrentMessageCache, descriptorID).PayloadDescriptor == message.PING_DESCRIPTOR {
		fmt.Println(myAddress, " - ping mesage already processed")
		return
	}

	msgForForwarding, err := messageCodec.EncodePingMessage(descriptorHeader)
	if err != nil {
		fmt.Println(myAddress, " - error writing the message for forwarding: ", err.Error())
	}

	SendMessageToAllConnectionsExcept(
		msgForForwarding, map[string]struct{}{common.GetIPFromConnection(conn): {}})

	SendNewPongMessage(conn, descriptorHeader.DescriptorID, descriptorHeader.Hops)
}

func ProcessPongMessage(conn net.Conn, descriptorHeader message.DescriptorHeader, pongMessage message.PongMessage) {
	descriptorID := string(descriptorHeader.DescriptorID[:])
	if !concurrentMessageCache.HasKey(descriptorID) {
		fmt.Println(myAddress, " - didn't receive any ping message with this descriptor ID, discarding message")
		return
	}

	if descriptorHeader.TTL == 0 {
		fmt.Println(myAddress, " - pong ttl zero - ", GetConnection(concurrentMessageCache, descriptorID) == nil)
		//use the new connection from the pong message

		return
	}

	msgForBackPropagating, err := messageCodec.EncodePongMessage(
		descriptorHeader,
		pongMessage,
	)
	if err != nil {
		fmt.Println(myAddress, " - error writing the body of the backpropagated pong message: ", err.Error())
		return
	}

	fmt.Println(myAddress, " - backpropagating: ", msgForBackPropagating)
	connToSend := GetConnection(concurrentMessageCache, descriptorID)
	connToSend.Write(PrependDataSize(msgForBackPropagating))
}

func ProcessQueryMessage(conn net.Conn, descriptorHeader message.DescriptorHeader, queryMessage message.QueryMessage) {
	if descriptorHeader.TTL == 0 {
		fmt.Println(myAddress, " - query message expired")
		return
	}

	descriptorID := string(descriptorHeader.DescriptorID[:])
	messageFound := concurrentMessageCache.AddUnique(descriptorID, CreatePair(conn, descriptorHeader))

	if messageFound &&
		GetDescriptorHeader(concurrentMessageCache, descriptorID).PayloadDescriptor == message.QUERY_DESCRIPTOR {
		fmt.Println(myAddress, " - query mesage already processed")
		return
	}

	msgForForwarding, err := messageCodec.EncodeQueryMessage(
		descriptorHeader,
		queryMessage,
	)
	if err != nil {
		fmt.Println(myAddress, " - error encoding the query message for forwarding: ", err.Error())
	}

	SendMessageToAllConnectionsExcept(
		msgForForwarding, map[string]struct{}{common.GetIPFromConnection(conn): {}})

	searchCriteria := fmt.Sprintf("%s", queryMessage.SearchCriteria)
	var resultSet []message.QueryResult
	//use query message to find files
	for _, fileName := range sharedFileNames {
		if strings.Contains(fileName, searchCriteria) {
			resultSet = append(resultSet,
				message.QueryResult{FileSize: 0, FileName: fileName})
		}
	}

	if len(resultSet) > 0 {
		SendNewQueryHitMessage(conn, descriptorHeader.DescriptorID, descriptorHeader.Hops, resultSet)
	}
}

func ProcessQueryHitMessage(conn net.Conn, descriptorHeader message.DescriptorHeader, queryHitMessage message.QueryHitMessage) {
	descriptorID := string(descriptorHeader.DescriptorID[:])
	if !concurrentMessageCache.HasKey(descriptorID) {
		fmt.Println(myAddress, " - didn't receive any query message with this descriptor ID, discarding message")
		return
	}

	if descriptorHeader.TTL == 0 {
		fmt.Println(myAddress, " - query hit ttl zero - ", GetConnection(concurrentMessageCache, descriptorID) == nil)
		//use the new connection from the pong message

		return
	}

	msgForBackPropagating, err := messageCodec.EncodeQueryHitMessage(
		descriptorHeader,
		queryHitMessage,
	)
	if err != nil {
		fmt.Println(myAddress, " - error encoding the backpropagated query hit message: ", err.Error())
		return
	}

	fmt.Println(myAddress, " - backpropagating: ", msgForBackPropagating)
	connToSend := GetConnection(concurrentMessageCache, descriptorID)
	connToSend.Write(PrependDataSize(msgForBackPropagating))
}

func ProcessMessage(conn net.Conn, msg []byte) {
	msgBuffer := bytes.NewBuffer(msg)
	descriptorHeader, err := messageCodec.DecodeDescriptorHeader(msgBuffer)
	if err != nil {
		fmt.Println(myAddress, " - error reading descriptor header: ", err.Error())
	}

	descriptorHeader.Hops++
	descriptorHeader.TTL--

	switch descriptorHeader.PayloadDescriptor {

	case message.PING_DESCRIPTOR:
		fmt.Println(myAddress, " - ping message received: ", descriptorHeader)
		ProcessPingMessage(conn, descriptorHeader)

	case message.PONG_DESCRIPTOR:
		pongMessage, err := messageCodec.DecodePongMessage(msgBuffer)
		if err != nil {
			fmt.Println(myAddress, "- error reading pong message: ", err.Error())
		}
		fmt.Println(myAddress, " - pong message received: ", pongMessage)
		ProcessPongMessage(conn, descriptorHeader, pongMessage)

	case message.QUERY_DESCRIPTOR:
		queryMessage, err := messageCodec.DecodeQueryMessage(msgBuffer)
		if err != nil {
			fmt.Println(myAddress, "- error reading query message: ", err.Error())
		}
		fmt.Println(myAddress, "- query message received: ", queryMessage, ", search criteria size: ", len(queryMessage.SearchCriteria))
		ProcessQueryMessage(conn, descriptorHeader, queryMessage)
	case message.QUERY_HIT_DESCRIPTOR:
		queryHitMessage, err := messageCodec.DecodeQueryHitMessage(msgBuffer)
		if err != nil {
			fmt.Println(myAddress, " - error reading query hit message: ", err.Error())
		}
		fmt.Println(myAddress, " - query hit message received: ", queryHitMessage)
		ProcessQueryHitMessage(conn, descriptorHeader, queryHitMessage)
	}

}

func WaitForMessagesFromConnection(conn net.Conn) {
	var buffer []byte
	needPrefixLength := true
	var requiredBufferLen uint32 = 4 // hardcoded
	for {
		newMessage := make([]byte, 1024) //hardcoded size
		numRead, err := conn.Read(newMessage)
		fmt.Println(myAddress, " - message received: ", newMessage[:numRead], " from - ", common.GetIPFromConnection(conn), "numRead: ", numRead)

		if err != nil {
			fmt.Println("Error receiving message from connection ", common.GetIPFromConnection(conn), ": ", err.Error())
		}
		buffer = append(buffer, newMessage[:numRead]...)
		for {
			if uint32(len(buffer)) >= requiredBufferLen {
				saveRequiredBufferLen := requiredBufferLen
				if needPrefixLength {
					requiredBufferLen = binary.LittleEndian.Uint32(buffer)
					needPrefixLength = false
				} else {
					ProcessMessage(conn, buffer[:requiredBufferLen])
					requiredBufferLen = 4 // hardcoded
					needPrefixLength = true
				}
				buffer = buffer[saveRequiredBufferLen:]
			} else {
				break
			}
		}
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

func Test1() {
	/*tmpResultSet := []QueryResult2{
		QueryResult2{FileSize: 30, FileName: [5]byte{'f', 'i', 'l', 'e', '1'}},
		QueryResult2{FileSize: 50, FileName: [5]byte{'f', 'i', 'l', 'e', '2'}},
		QueryResult2{FileSize: 90, FileName: [5]byte{'f', 'i', 'l', 'e', '3'}},
	}
	//resultSetSizeInBytes := GetQueryHitResultSetSizeInBytes(tmpResultSet)

	//fmt.Println(resultSetSizeInBytes)

	var tmpBuffer bytes.Buffer
	binary.Write(&tmpBuffer, binary.LittleEndian, tmpResultSet)
	fmt.Println("written result - ", tmpBuffer.Bytes())

	var tmpReadResultSet QueryResult2
	binary.Read(&tmpBuffer, binary.LittleEndian, &tmpReadResultSet)

	fmt.Println("read result - ", tmpReadResultSet)*/
}

func Test2() {
	/*tmpResultSet := []QueryResult{
		QueryResult{FileSize: 30, FileName: []byte("file1")},
		QueryResult{FileSize: 50, FileName: []byte("file2")},
		QueryResult{FileSize: 90, FileName: []byte("file3")},
	}
	//resultSetSizeInBytes := GetQueryHitResultSetSizeInBytes(tmpResultSet)

	//fmt.Println(resultSetSizeInBytes)

	var tmpBuffer bytes.Buffer
	encoder := gob.NewEncoder(&tmpBuffer)
	decoder := gob.NewDecoder(&tmpBuffer)

	encoder.Encode(tmpResultSet)
	fmt.Println("written result - ", tmpBuffer.Bytes())

	var tmpReadResultSet []QueryResult
	decoder.Decode(&tmpReadResultSet)

	fmt.Println("read result - ", tmpReadResultSet)*/
}

func Test3() {
	tmp := "test"

	var tmpBuffer bytes.Buffer
	binary.Write(&tmpBuffer, binary.LittleEndian, &tmp)

	var tmpRead string
	binary.Read(&tmpBuffer, binary.LittleEndian, &tmpRead)

	fmt.Println("read: ", tmpRead)
}

func Test4() {

	var m message.MessageCodec = new(message.MessageCodecGob)

	tmpResultSet := []message.QueryResult{
		message.QueryResult{FileSize: 30, FileName: "file1"},
		message.QueryResult{FileSize: 50, FileName: "file2"},
		message.QueryResult{FileSize: 90, FileName: "file3"},
	}

	msg := message.QueryHitMessage{
		NumberOfHits: 3,
		Port:         1000,
		IPAddress:    [4]byte{1, 2, 3, 4},
		Speed:        10,
		ResultSet:    tmpResultSet,
	}

	fmt.Println("        message - ", msg)

	data, err := m.EncodeQueryHitMessage(message.DescriptorHeader{}, msg)
	if err != nil {
		fmt.Println("Error encoding message")
	}

	buffer := bytes.NewBuffer(data)
	decodedQueryHitMessage, err := m.DecodeQueryHitMessage(buffer)
	if err != nil {
		fmt.Println("Error decoding message")
	}

	/*buffer := bytes.NewBuffer(data)
	var decodedQueryHitMessage message.QueryHitMessage
	err = message.Decode(buffer, &decodedQueryHitMessage)
	if err != nil {
		fmt.Println("Error decoding message")
	}*/

	fmt.Println("decoded message - ", decodedQueryHitMessage)
}

func Test5() {
	buffer := []byte{2, 5, 1, 7}
	//fmt.Println(buffer[:4])
	//fmt.Println(buffer[4:])

	fmt.Println(buffer[:len(buffer)])
	fmt.Println(buffer[len(buffer):])
}

func TestSubSlice() {
	f := func(s []int) {
		s[0] = 3
		s[1] = 10
	}

	s := []int{1, 2, 3, 4, 5}
	f(s[1:])

	f2 := func(s []int, val int) {
		s[0] = val
		s[1] = val
	}

	s2 := []int{10, 21, 35, 41, 53}

	fmt.Println("s2 before: ", s2)
	f2(s2[1:], 78)
	f2(s2[3:], 19)

	fmt.Println("s2 after: ", s2)
}

func main() {
	fmt.Println(myAddress, " - my files: ", sharedFileNames)

	//Test5()
	//TestSubSlice()

	//return

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
