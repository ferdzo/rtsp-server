package main

import (
	"fmt"
	"net"
)

const (
	rtpPort = 5004
)

func main() {
	listener, err := net.Listen("tcp", ":554")
	if err != nil {
		fmt.Println("Error starting server:", err)
		return
	}
	defer func(listener net.Listener) {
		err := listener.Close()
		if err != nil {

		}
	}(listener)

	fmt.Println("RTSP server listening on :554")
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}
		go handleRTSP(conn)
	}
}
