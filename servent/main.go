package servent

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sp2pfs/message"
	"sp2pfs/util"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const PING_INTERVAL = 10
const MIN_NR_KILOBYTES_FOR_CONN = 100000
const MIN_NR_SHARED_FILES_FOR_CONN = 100

var myAddress string = GetMyAddress().String()

var sharedFileNames []string = CreateFileNames()

var messageCodec message.MessageCodec = new(message.MessageCodecBasic)

//var connections map[string]net.Conn = make(map[string]net.Conn)                      // shared, writable, needs sync
//var concurrentMessageCache *util.ConcurrentStringMap = util.NewConcurrentStringMap() // shared, writable, needs sync

type Servent struct {
	//connections            map[string]net.Conn       //= make(map[string]net.Conn) // shared, writable, needs sync
	connections            *util.ConcurrentStringMap
	concurrentMessageCache *util.ConcurrentStringMap //= util.NewConcurrentStringMap() // shared, writable, needs sync
	concurrentFileCache    *util.ConcurrentStringMap
	connectServiceAddress  string
	sharedFilesPath        string
	queryResponseHandler   func(message.QueryHitMessage)
	downloadServerRouter   *gin.Engine
}

func NewServent(connectServiceAddress, sharedFilesPath string) *Servent {
	var servent Servent

	//servent.connections = make(map[string]net.Conn)
	servent.connections = util.NewConcurrentStringMap()
	servent.concurrentMessageCache = util.NewConcurrentStringMap()
	servent.concurrentFileCache = util.NewConcurrentStringMap()
	servent.connectServiceAddress = connectServiceAddress
	servent.sharedFilesPath = sharedFilesPath
	servent.queryResponseHandler = nil
	servent.downloadServerRouter = gin.Default()
	gin.SetMode(gin.ReleaseMode)

	return &servent
}

func getConnection(concurrentMessageCache *util.ConcurrentStringMap, key string) net.Conn { //should also return error

	value, keyFound := concurrentMessageCache.Get(key)
	valuePair := value.(util.Pair)
	if !keyFound || valuePair.Values[0] == nil {
		return nil
	}

	return valuePair.Values[0].(net.Conn)
}

func getDescriptorHeader(concurrentMessageCache *util.ConcurrentStringMap, key string) message.DescriptorHeader { //should also return error
	value, _ := concurrentMessageCache.Get(key)
	valuePair := value.(util.Pair)

	return valuePair.Values[1].(message.DescriptorHeader) //what if the conversion fails
}

func PrependDataSize(data []byte) []byte {
	prefix := make([]byte, 4) // hardcoded
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

func GetNumberOfKilobytes(nrOfBytes uint32) float64 {
	return (float64(nrOfBytes) / 1024.0)
}

func /*(Servent)*/ createDescriptorHeader(descriptorId [16]byte, payloadDescriptor, ttl, hops uint8, payloadLength uint32) []byte {
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

func (servent *Servent) write(conn net.Conn, message []byte) (int, error) {
	numWritten, err := conn.Write(message)
	if err != nil {
		servent.connections.Delete(util.GetIPFromConnection(conn))
		fmt.Println(myAddress, "- my connections: ", servent.connections)
		return numWritten, err // or 0, err?
	}
	return numWritten, nil
}

func (servent *Servent) read(conn net.Conn, buffer []byte) (int, error) {
	numRead, err := conn.Read(buffer)
	if err != nil {
		servent.connections.Delete(util.GetIPFromConnection(conn))
		fmt.Println(myAddress, "- my connections: ", servent.connections)
		return numRead, err // or 0, err?
	}
	return numRead, nil
}

func (servent *Servent) createPingMessage() ([]byte, error) {
	descriptorID := uuid.New()

	descriptorHeader := message.DescriptorHeader{
		DescriptorID:      descriptorID,
		PayloadDescriptor: message.PING_DESCRIPTOR,
		TTL:               7,
		Hops:              0,
	}

	pingMessageData, err := messageCodec.EncodePingMessage(descriptorHeader)

	if err != nil {
		return nil, err
	}

	servent.concurrentMessageCache.Set(string(descriptorID[:]), util.CreatePair(nil, descriptorHeader)) // synchronization?

	return pingMessageData, nil
}

func (servent *Servent) createPongMessage(descriptorID [16]byte, ttl uint8) ([]byte, error) {
	port, err := strconv.ParseUint(os.Getenv("SERVENT_PORT"), 10, 16)
	if err != nil {
		return nil, err
	}

	ipAddr := GetMyAddress()

	numberOfSharedFiles := servent.concurrentFileCache.Len()
	var numberOfKilobytes float64 = 0.0
	servent.concurrentFileCache.ApplyOperation(func(path interface{}, size interface{}) {
		numberOfKilobytes += GetNumberOfKilobytes(size.(uint32))
	})

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
			NumberOfSharedFiles:     uint32(numberOfSharedFiles),
			NumberOfKilobytesShared: uint32(math.Ceil(numberOfKilobytes)), //can this conversion fail?
		},
	)

	if err != nil {
		return nil, err
	}

	return pongMessageData, nil
}

func (servent *Servent) createQueryMessage(minimumSpeed uint16, searchCriteria string) ([]byte, error) {
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
		return nil, err
	}

	servent.concurrentMessageCache.Set(string(descriptorID[:]), util.CreatePair(nil, descriptorHeader)) // synchronization?

	return queryMessageData, nil
}

func (servent *Servent) createQueryHitMessage(descriptorID [16]byte, ttl uint8, resultSet []message.QueryResult) ([]byte, error) {
	port, err := strconv.ParseUint(os.Getenv("SERVENT_DOWNLOAD_PORT"), 10, 16)
	if err != nil {
		return nil, err
	}

	numberOfHits := uint8(len(resultSet))
	ipAddr := GetMyAddress()

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
		return nil, err
	}

	return queryHitMessageData, nil
}

func (servent *Servent) sendNewPingMessage() error {
	pingMessage, err := servent.createPingMessage()
	if err != nil {
		return err
	}

	servent.connections.ApplyOperation(func(_ interface{}, conn interface{}) {
		servent.write(conn.(net.Conn), PrependDataSize(pingMessage))
	})

	return nil
}

func (servent *Servent) sendMessageToAllConnectionsExcept(message []byte, exclusionMap map[string]struct{}) {
	servent.connections.ApplyOperation(func(ipAddress interface{}, conn interface{}) {
		if _, ok := exclusionMap[ipAddress.(string)]; ok == false {
			servent.write(conn.(net.Conn), PrependDataSize(message))
		}
	})
}

func (servent *Servent) sendNewPongMessage(conn net.Conn, descriptorID [16]byte, ttl uint8) error {
	pongMessage, err := servent.createPongMessage(descriptorID, ttl)
	if err != nil {
		return err
	}

	servent.write(conn, PrependDataSize(pongMessage))

	return nil
}

func (servent *Servent) sendPingPeriodically() {
	ticker := time.NewTicker(PING_INTERVAL * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			servent.sendNewPingMessage()
		}
	}
}

func (servent *Servent) sendNewQueryMessage(minimumSpeed uint16, searchCriteria string) error {
	queryMessageData, err := servent.createQueryMessage(minimumSpeed, searchCriteria)
	if err != nil {
		return err
	}

	servent.connections.ApplyOperation(func(_ interface{}, conn interface{}) {
		servent.write(conn.(net.Conn), PrependDataSize(queryMessageData))
	})

	return nil
}

func (servent *Servent) sendNewQueryHitMessage(conn net.Conn, descriptorID [16]byte, ttl uint8, resultSet []message.QueryResult) error {
	queryHitMessage, err := servent.createQueryHitMessage(descriptorID, ttl, resultSet)
	if err != nil {
		return err
	}

	servent.write(conn, PrependDataSize(queryHitMessage))

	return nil
}

func (servent *Servent) addConnectionAndStartWaitingForMessages(ip string, conn net.Conn) {
	if connectionFound := servent.connections.AddUnique(ip, conn); !connectionFound {
		go servent.waitForMessagesFromConnection(conn)
	}
}

func (servent *Servent) processPingMessage(conn net.Conn, descriptorHeader message.DescriptorHeader) error {
	if descriptorHeader.TTL == 0 { //ping message expired
		return nil
	}

	descriptorID := string(descriptorHeader.DescriptorID[:])
	messageFound := servent.concurrentMessageCache.AddUnique(descriptorID, util.CreatePair(conn, descriptorHeader))

	if messageFound &&
		getDescriptorHeader(servent.concurrentMessageCache, descriptorID).PayloadDescriptor == message.PING_DESCRIPTOR { //ping mesage already processed
		return nil
	}

	msgForForwarding, err := messageCodec.EncodePingMessage(descriptorHeader)
	if err != nil {
		return err
	}

	servent.sendMessageToAllConnectionsExcept(
		msgForForwarding, map[string]struct{}{util.GetIPFromConnection(conn): {}})

	err = servent.sendNewPongMessage(conn, descriptorHeader.DescriptorID, descriptorHeader.Hops)
	if err != nil {
		return err
	}

	return nil
}

func (servent *Servent) processPongMessage(conn net.Conn, descriptorHeader message.DescriptorHeader, pongMessage message.PongMessage) error {
	descriptorID := string(descriptorHeader.DescriptorID[:])
	if !servent.concurrentMessageCache.HasKey(descriptorID) { //didn't receive any ping message with this descriptor ID, discarding message
		return nil
	}

	if descriptorHeader.TTL == 0 { //pong ttl zero, should be at original ping sender
		//use the new connection from the pong message
		if pongMessage.NumberOfKilobytesShared > MIN_NR_KILOBYTES_FOR_CONN ||
			pongMessage.NumberOfSharedFiles > MIN_NR_SHARED_FILES_FOR_CONN {
			ipAddress := util.ConvertBytesToIpAddress(pongMessage.IPAddress)
			if !servent.connections.HasKey(ipAddress) { //trying to connect to servent from pong message
				conn, err := net.Dial("tcp", ipAddress+":"+strconv.FormatUint(uint64(pongMessage.Port), 10))
				if err != nil {
					//TODO(?): maybe handle this case in some way
				} else { //connected to the servent from the pong message
					servent.addConnectionAndStartWaitingForMessages(ipAddress, conn)
				}
			}
		}

		return nil
	}

	msgForBackPropagating, err := messageCodec.EncodePongMessage(
		descriptorHeader,
		pongMessage,
	)
	if err != nil { //error encoding the backpropagated pong message
		return err
	}

	connToSend := getConnection(servent.concurrentMessageCache, descriptorID)
	servent.write(connToSend, PrependDataSize(msgForBackPropagating))

	return nil
}

func (servent *Servent) processQueryMessage(conn net.Conn, descriptorHeader message.DescriptorHeader, queryMessage message.QueryMessage) error {

	if descriptorHeader.TTL == 0 { //query message expired
		return nil
	}

	descriptorID := string(descriptorHeader.DescriptorID[:])
	messageFound := servent.concurrentMessageCache.AddUnique(descriptorID, util.CreatePair(conn, descriptorHeader))

	if messageFound &&
		getDescriptorHeader(servent.concurrentMessageCache, descriptorID).PayloadDescriptor == message.QUERY_DESCRIPTOR { //query mesage already processed
		return nil
	}

	msgForForwarding, err := messageCodec.EncodeQueryMessage(
		descriptorHeader,
		queryMessage,
	)
	if err != nil { //error encoding the query message for forwarding
		return err
	}

	servent.sendMessageToAllConnectionsExcept(
		msgForForwarding, map[string]struct{}{util.GetIPFromConnection(conn): {}})

	var resultSet []message.QueryResult
	servent.concurrentFileCache.ApplyOperation(func(path interface{}, size interface{}) {
		if strings.Contains(path.(string), queryMessage.SearchCriteria) {
			resultSet = append(resultSet, message.QueryResult{FileSize: size.(uint32), FileName: path.(string)})
		}
	})

	if len(resultSet) > 0 {
		servent.sendNewQueryHitMessage(conn, descriptorHeader.DescriptorID, descriptorHeader.Hops, resultSet)
	}

	return nil
}

func (servent *Servent) processQueryHitMessage(conn net.Conn, descriptorHeader message.DescriptorHeader, queryHitMessage message.QueryHitMessage) error {
	descriptorID := string(descriptorHeader.DescriptorID[:])
	if !servent.concurrentMessageCache.HasKey(descriptorID) { //didn't receive any query message with this descriptor ID, discarding message
		return nil
	}

	if descriptorHeader.TTL == 0 { //query hit ttl zero, should be at original sender of query
		servent.queryResponseHandler(queryHitMessage)
		return nil
	}

	msgForBackPropagating, err := messageCodec.EncodeQueryHitMessage(
		descriptorHeader,
		queryHitMessage,
	)
	if err != nil { //error encoding the backpropagated query hit message
		return err
	}

	connToSend := getConnection(servent.concurrentMessageCache, descriptorID)
	servent.write(connToSend, PrependDataSize(msgForBackPropagating))

	return nil
}

func (servent *Servent) processMessage(conn net.Conn, msg []byte) error {
	msgBuffer := bytes.NewBuffer(msg)
	descriptorHeader, err := messageCodec.DecodeDescriptorHeader(msgBuffer)
	if err != nil { //error reading descriptor header
		return err
	}

	descriptorHeader.Hops++
	descriptorHeader.TTL--

	switch descriptorHeader.PayloadDescriptor {

	case message.PING_DESCRIPTOR:
		err = servent.processPingMessage(conn, descriptorHeader)
		if err != nil {
			return err
		}

	case message.PONG_DESCRIPTOR:
		pongMessage, err := messageCodec.DecodePongMessage(msgBuffer)
		if err != nil {
			return err
		}

		err = servent.processPongMessage(conn, descriptorHeader, pongMessage)
		if err != nil {
			return err
		}

	case message.QUERY_DESCRIPTOR:
		queryMessage, err := messageCodec.DecodeQueryMessage(msgBuffer)
		if err != nil {
			return err
		}

		err = servent.processQueryMessage(conn, descriptorHeader, queryMessage)
		if err != nil {
			return err
		}

	case message.QUERY_HIT_DESCRIPTOR:
		queryHitMessage, err := messageCodec.DecodeQueryHitMessage(msgBuffer)
		if err != nil {
			return err
		}

		err = servent.processQueryHitMessage(conn, descriptorHeader, queryHitMessage)
		if err != nil {
			return err
		}
	}

	return nil
}

func (servent *Servent) waitForMessagesFromConnection(conn net.Conn) {
	var buffer []byte
	needPrefixLength := true
	var requiredBufferLen uint32 = 4 // hardcoded
	for {
		newMessage := make([]byte, 1024) //hardcoded size
		numRead, err := servent.read(conn, newMessage)

		if err != nil {
			fmt.Println(myAddress, " - error receiving message from connection ", util.GetIPFromConnection(conn), ": ", err.Error())
			break
		}
		buffer = append(buffer, newMessage[:numRead]...)
		for {
			if uint32(len(buffer)) >= requiredBufferLen {
				saveRequiredBufferLen := requiredBufferLen
				if needPrefixLength {
					requiredBufferLen = binary.LittleEndian.Uint32(buffer)
					needPrefixLength = false
				} else {
					err := servent.processMessage(conn, buffer[:requiredBufferLen])
					if err != nil { //probably should handle differently
						fmt.Println(myAddress, " - error processing message: ", err.Error())
						break
					}
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

func (servent *Servent) waitForConnections() {
	listener, err := net.Listen("tcp", ":"+os.Getenv("SERVENT_PORT"))

	if err != nil {
		fmt.Println(myAddress, " - error creating servent listener:", err.Error())
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println(myAddress, " - error accepting new connection: ", err.Error())
		} else {
			servent.addConnectionAndStartWaitingForMessages(util.GetIPFromConnection(conn), conn)
			fmt.Println(myAddress, " - accepted connection ", conn.RemoteAddr())
		}
	}
}

func (servent *Servent) watchSharedFilePath() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Println(myAddress, " - error creating new watcher: ", err.Error())
		return
	}
	defer watcher.Close()

	err = filepath.Walk(servent.sharedFilesPath, func(path string, fileInfo os.FileInfo, err error) error {
		if fileInfo.IsDir() {
			return watcher.Add(path)
		}
		servent.concurrentFileCache.AddUnique(path, uint32(fileInfo.Size()))
		return nil
	})
	if err != nil {
		fmt.Println(myAddress, " - error walking the file path: ", err.Error())
		return
	}

	for {
		select {
		case event := <-watcher.Events:
			fmt.Println(myAddress, " - file event: ", event)

			switch event.Op {
			case fsnotify.Remove, fsnotify.Rename:
				servent.concurrentFileCache.Delete(event.Name)

			case fsnotify.Create, fsnotify.Write:
				fileInfo, err := os.Stat(event.Name)
				if err != nil {
					fmt.Println(myAddress, " - error accessing file: ", err.Error())
					return
				}
				if event.Op == fsnotify.Create {
					if fileInfo.IsDir() {
						watcher.Add(event.Name)
					} else {
						servent.concurrentFileCache.AddUnique(event.Name, uint32(fileInfo.Size()))
					}
				} else {
					servent.concurrentFileCache.Set(event.Name, uint32(fileInfo.Size()))
				}
			}

		case err = <-watcher.Errors:
			fmt.Println(myAddress, " - error watching the dirs: ", err.Error())
			return
		}
	}
}

func (servent *Servent) startDownloadServer() {
	servent.downloadServerRouter.GET("/file", func(c *gin.Context) {
		path := c.Query("path")
		c.File(path)
	})
	servent.downloadServerRouter.Run(":" + os.Getenv("SERVENT_DOWNLOAD_PORT"))
}

func (servent *Servent) SetQueryResponseHandler(handler func(message.QueryHitMessage)) {
	servent.queryResponseHandler = handler
}

func (servent *Servent) Start() {
	connectServiceConn, err := net.Dial("tcp", servent.connectServiceAddress)

	if err != nil {
		fmt.Println(myAddress, " - error dialing connect service: ", err.Error())
		return
	}
	defer connectServiceConn.Close()

	successfullyConnected := false

	serventListData := make([]byte, 1024)
	numRead, err := connectServiceConn.Read(serventListData)
	if err != nil {
		fmt.Println(myAddress, " - error reading servent list from connect service: ", err.Error())
		return
	}

	serventListData = append([]byte(nil), serventListData[:numRead]...)
	var serventList []util.Address
	err = json.Unmarshal(serventListData, &serventList)
	if err != nil {
		fmt.Println(myAddress, " - error unmarshalling servent list: ", err.Error())
		return
	}

	if len(serventList) == 0 {
		fmt.Println(myAddress, " - first servent")
		successfullyConnected = true
	}

	for _, crntServent := range serventList {
		conn, err := net.Dial("tcp", crntServent.Host+":"+crntServent.Port)
		if err == nil {
			servent.addConnectionAndStartWaitingForMessages(crntServent.Host, conn)
			successfullyConnected = true
		}
	}

	var port string = "0"
	if successfullyConnected {
		port = os.Getenv("SERVENT_PORT")
	}

	portData, err := json.Marshal(port)
	if err != nil {
		fmt.Println(myAddress, " - error marshalling port number: ", err.Error())
	}

	_, err = connectServiceConn.Write(portData)
	if err != nil {
		fmt.Println(myAddress, " - error sending port to connect service: ", err.Error())
	}

	go servent.sendPingPeriodically()

	go servent.startDownloadServer()

	go servent.watchSharedFilePath() // maybe use watcher to not walk the file path every time

	servent.waitForConnections()
}

func (servent *Servent) Query(minimumSpeed uint16, searchCriteria string) error {
	if servent.queryResponseHandler == nil {
		return errors.New(myAddress + " - error: query response handler not set")
	}
	servent.sendNewQueryMessage(minimumSpeed, searchCriteria)

	return nil
}

func (servent *Servent) Download(fileToDownload, filename, ipAddress string) error {
	queryEscapedFileToDownload := url.QueryEscape(fileToDownload)
	url := "http://" + ipAddress + "/file?path=" + queryEscapedFileToDownload

	resp, err := http.Get(url)
	if err != nil {
		return err
	}

	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	ioutil.WriteFile(servent.sharedFilesPath+"/"+filename, responseBody, 0644)

	return nil
}
