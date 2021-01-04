package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"sp2pfs/message"
	"sp2pfs/servent"
)

func applyCommand(command string, s *servent.Servent) {
	words := strings.Fields(command)
	if len(words) == 0 {
		return
	}

	switch words[0] {
	case "query":
		if len(words) == 2 {
			s.Query(0, words[1])
		}
	case "download":
		if len(words) == 4 {
			s.Download(words[1], words[2], words[3])
		}
	default:
		fmt.Println("Unknown command")
	}
}

func readServentCommands(fileDescriptor *os.File, s *servent.Servent) {
	scanner := bufio.NewScanner(fileDescriptor)
	for {
		fmt.Print(">>>")
		scanner.Scan()
		applyCommand(scanner.Text(), s)
	}
}

func main() {
	interactiveFlag := flag.Bool("it", false, "Read servent commands from the command line")
	connectServiceAddress := flag.String("addr", "", "The address of the connect service server")
	sharedFilesPath := flag.String("files", "", "The path to the files that are shared by the servent")
	flag.Parse()

	if *connectServiceAddress == "" || *sharedFilesPath == "" {
		fmt.Println("Error: Connect service address and shared files path are required")
		flag.PrintDefaults()
		return
	}

	servent := servent.NewServent(*connectServiceAddress, *sharedFilesPath)
	servent.SetQueryResponseHandler(func(queryResult message.QueryHitMessage) {
		fmt.Println("received query results: ", queryResult)
	})

	if *interactiveFlag {
		go servent.Start()
		readServentCommands(os.Stdin, servent)
	} else {
		servent.Start()
	}
}
