package main

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net"
	"os"
	"strings"
	"sync"

	"sp2pfs/common"
)

const BUFF_SIZE = 1024
const NUM_SVTS_FOR_CONN = 2

var currentServentList AddressSyncSlice

type AddressSyncSlice struct {
	slice []common.Address
	sync.RWMutex
}

func (addressSyncSlice *AddressSyncSlice) Append(address common.Address) {
	addressSyncSlice.Lock()
	defer addressSyncSlice.Unlock()

	addressSyncSlice.slice = append(addressSyncSlice.slice, address)
}

func sendCurrentServentList(conn net.Conn) {
	currentServentList.Lock()
	data, err := json.Marshal(currentServentList.slice)
	if err != nil {
		fmt.Println("Error marshaling servent list: ", err.Error())
		return
	}

	numWritten, err := conn.Write(data)
	if err != nil {
		fmt.Println("Error sending servent list: ", err.Error())
		return
	}
	if numWritten != len(data) {
		fmt.Println("Servent list data only partially sent")
	}
	currentServentList.Unlock()
}

func sendServentSublist(conn net.Conn) {
	defer conn.Close()

	currentServentList.Lock()
	currentServentListLen := len(currentServentList.slice)
	currentServentList.Unlock()

	var indexList []int
	for i := 0; i < currentServentListLen; i++ {
		indexList = append(indexList, i)
	}

	removeFromSlice := func(slice []int, index int) []int {
		return append(slice[:index], slice[index+1:]...)
	}

	numAddrToSend := int(math.Min(float64(currentServentListLen), float64(NUM_SVTS_FOR_CONN)))
	var addressSliceToSend []common.Address
	for i := 0; i < numAddrToSend; i++ {
		randomIndex := rand.Intn(len(indexList))
		addressSliceToSend = append(addressSliceToSend, currentServentList.slice[randomIndex]) //No need for sync, elements are added at the end of the address slice
		indexList = removeFromSlice(indexList, randomIndex)
	}
	data, err := json.Marshal(addressSliceToSend)

	numWritten, err := conn.Write(data)
	if err != nil {
		fmt.Println("Error sending servent list: ", err.Error())
		return
	}
	if numWritten != len(data) {
		fmt.Println("Servent list data only partially sent")
	}

	response := make([]byte, BUFF_SIZE)
	numRead, err := conn.Read(response)
	response = append([]byte(nil), response[:numRead]...)

	if err != nil {
		fmt.Println("Error reading response from servent: ", err.Error())
	} else {
		var port string
		json.Unmarshal(response, &port)

		if port != "0" {
			currentServentList.Append(common.Address{Host: strings.Split(conn.RemoteAddr().String(), ":")[0], Port: port})
		} else {
			fmt.Println("Servent couldn't connect, not adding it to the list")
		}
	}
}

func main() {

	l, err := net.Listen("tcp", ":"+os.Getenv("CONNECT_SERVICE_PORT"))
	if err != nil {
		fmt.Println("Error listening: ", err.Error())
		os.Exit(1)
	}
	defer l.Close()

	fmt.Println("Waiting for connections...")
	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting:", err.Error())
			os.Exit(1)
		}
		go sendServentSublist(conn)
		fmt.Println(strings.Split(conn.RemoteAddr().String(), ":")[0])
	}
}
