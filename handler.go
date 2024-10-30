package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"
)

func handleRTSP(conn net.Conn) {
	defer func() {
		if err := conn.Close(); err != nil {
			fmt.Println("Error closing connection:", err)
		}
	}()

	var cseq int                           // Initialize CSeq
	var transportType string               // To store transport type (TCP or UDP)
	var clientIP string                    // To store client IP address
	var clientPortStart, clientPortEnd int // Client ports for RTP

	reader := bufio.NewReader(conn)
	for {
		var request strings.Builder
		clientIP = conn.RemoteAddr().String()
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					// Client has closed the connection; log and exit gracefully
					fmt.Println("Client closed the connection:", conn.RemoteAddr())
					return
				}
				fmt.Println("Error reading:", err)
				return

			}
			request.WriteString(line)
			if line == "\r\n" { // End of request
				break
			}
		}

		reqStr := request.String()
		fmt.Println("RTSP Request:", reqStr)

		if strings.Contains(reqStr, "CSeq:") {
			cseq = extractCSeq(reqStr)
		}

		if strings.Contains(reqStr, "OPTIONS") {
			cseq++
			handleOptions(conn, cseq)
			continue
		}

		if strings.Contains(reqStr, "DESCRIBE") {
			cseq++
			handleDescribe(conn, cseq, clientIP)
			continue
		}

		if strings.Contains(reqStr, "SETUP") {
			cseq++
			err := handleSetup(conn, reqStr, clientIP, cseq)
			if err != nil {
				return
			}
			continue
		}

		if strings.Contains(reqStr, "PLAY") {
			cseq++
			handlePlay(conn, cseq, clientIP, clientPortStart, clientPortEnd, transportType)
		}
		if strings.Contains(reqStr, "TEARDOWN") {
			cseq++
			handleTeardown(conn, reqStr, cseq)
			continue
		}
	}
}

func handleOptions(conn net.Conn, cseq int) {
	optionsResponse := fmt.Sprintf("RTSP/1.0 200 OK\r\nCSeq: %d\r\nPublic: OPTIONS, DESCRIBE, SETUP, TEARDOWN, PLAY, PAUSE\r\n\r\n", cseq)
	writeResponse(conn, optionsResponse)

}

func handleDescribe(conn net.Conn, cseq int, clientIP string) {
	describeResponse := fmt.Sprintf(`RTSP/1.0 200 OK
			CSeq: %d
			Content-Base: rtsp://%s:554/
			Content-Type: application/sdp
			Content-Length: 135
			
			v=0
			o=- 0 0 IN IP4 127.0.0.1
			s=JPEG Stream
			c=IN IP4 127.0.0.1
			t=0 0
			a=tool:golang-rtsp-server
			m=video %d RTP/AVP 26
			a=rtpmap:26 H264/90000
			`, cseq, clientIP, rtpPort)
	writeResponse(conn, describeResponse)

}

func handleSetup(conn net.Conn, reqStr string, clientIP string, cseq int) error {
	_, clientPortStart, clientPortEnd, err := parseTransportLine(reqStr)
	if err != nil {
		unsupportedResponse := fmt.Sprintf("RTSP/1.0 461 Unsupported Transport\r\nCSeq: %d\r\n\r\n", cseq)
		writeResponse(conn, unsupportedResponse)
		return err
	}

	setupResponse := fmt.Sprintf("RTSP/1.0 200 OK\r\nCSeq: %d\r\nTransport: RTP/AVP;unicast;client_port=%d-%d\r\nSession: 12345678\r\n\r\n", cseq, clientPortStart, clientPortEnd)
	writeResponse(conn, setupResponse)
	go videoStreamUDP(clientIP, clientPortStart)
	go sendRTCPReport(clientIP, clientPortEnd)

	return nil
}

func handlePlay(conn net.Conn, cseq int, clientIP string, clientPortStart int, clientPortEnd int, transportType string) {
	playResponse := fmt.Sprintf("RTSP/1.0 200 OK\r\nCSeq: %d\r\nSession: 12345678\r\n\r\n", cseq)
	writeResponse(conn, playResponse)

	if transportType == "UDP" {
		go streamImageUDP(clientIP, clientPortStart)
		go sendRTCPReport(clientIP, clientPortEnd)
	} else {
		playErrorResponse := fmt.Sprintf("RTSP/1.0 454 Session Not Found\r\nCSeq: %d\r\n\r\n", cseq)
		writeResponse(conn, playErrorResponse)
	}
}

func handleTeardown(conn net.Conn, reqStr string, cseq int) {
	var sessionID string
	if strings.Contains(reqStr, "Session:") {
		sessionID = extractSessionID(reqStr)
	}

	teardownResponse := fmt.Sprintf("RTSP/1.0 200 OK\r\nCSeq: %d\r\nSession: %s\r\n\r\n", cseq, sessionID)
	if _, err := conn.Write([]byte(teardownResponse)); err != nil {
		fmt.Println("Error writing TEARDOWN response:", err)
	}

	fmt.Println("TEARDOWN received, session stopped:", sessionID)

}

func writeResponse(conn net.Conn, response string) {
	if _, err := (conn).Write([]byte(response)); err != nil {
		fmt.Println("Error writing response:", err)
	}
}
