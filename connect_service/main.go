package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"

	"sp2pfs/util"
)

const BUFF_SIZE = 1024
const NUM_SVTS_FOR_CONN = 2

var currentServentList util.ConcurrentSlice

func sendServentSublist(conn net.Conn) error {
	randomAddressSlice := currentServentList.GetRandomValues(NUM_SVTS_FOR_CONN)
	fmt.Println("Servent list: ", currentServentList)
	fmt.Println("Servent list being sent ", randomAddressSlice, " to: ", conn.RemoteAddr())

	var addressSliceToSend []util.Address
	for _, address := range randomAddressSlice {
		addressSliceToSend = append(addressSliceToSend, address.(util.Address))
	}

	data, err := json.Marshal(addressSliceToSend)

	numWritten, err := conn.Write(data)
	if err != nil {
		return err
	}
	if numWritten != len(data) { // maybe need to handle this case
		fmt.Println("Servent list data only partially sent")
	}

	return nil
}

func waitForPortMessage(conn net.Conn) (string, error) {
	buffer := make([]byte, BUFF_SIZE)
	numRead, err := conn.Read(buffer) //TODO: need to address receive for multiple messages, even though that shouldn't happen
	if err != nil {
		return "", err
	}

	buffer = append([]byte(nil), buffer[:numRead]...)

	var port string
	err = json.Unmarshal(buffer, &port)
	if err != nil {
		return "", err
	}

	return port, nil
}

func handleNewServentConnection(conn net.Conn) {
	err := sendServentSublist(conn)
	if err != nil {
		fmt.Println("Error sending servent list: ", err.Error())
		return
	}

	port, err := waitForPortMessage(conn)
	if err != nil {
		fmt.Println("Error receiving port from servent: ", err.Error())
		return
	}
	if port == "0" {
		fmt.Println("Error - servent couldn't connect, not adding it to the list")
		return
	}

	servent := util.Address{Host: strings.Split(conn.RemoteAddr().String(), ":")[0], Port: port}
	fmt.Println("Servent - ", servent, " port: ", port)

	currentServentList.Append(servent)

	//wait for servent to close and remove from the list
	buffer := make([]byte, 1)
	_, err = conn.Read(buffer)
	if err != nil {
		fmt.Println("Removing servent from list: ", servent)
		currentServentList.DeleteValue(servent)
	}
	conn.Close()
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
		go handleNewServentConnection(conn)
		fmt.Println(strings.Split(conn.RemoteAddr().String(), ":")[0])
	}
}
