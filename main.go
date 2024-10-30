package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	rtpPort = 5004
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
			optionsResponse := fmt.Sprintf("RTSP/1.0 200 OK\r\nCSeq: %d\r\nPublic: OPTIONS, DESCRIBE, SETUP, TEARDOWN, PLAY, PAUSE\r\n\r\n", cseq)
			if _, err := conn.Write([]byte(optionsResponse)); err != nil {
				fmt.Println("Error writing OPTIONS response:", err)
			}
			continue
		}

		if strings.Contains(reqStr, "DESCRIBE") {
			cseq++
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

			if _, err := conn.Write([]byte(describeResponse)); err != nil {
				fmt.Println("Error writing DESCRIBE response:", err)
			}
			continue
		}

		if strings.Contains(reqStr, "SETUP") {
			cseq++

			// Extract transport and client port range
			if strings.Contains(reqStr, "Transport:") {
				transportLine := strings.Split(reqStr, "Transport:")[1]
				transportLine = strings.TrimSpace(transportLine)

				parts := strings.Split(transportLine, ";")
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if strings.HasPrefix(part, "client_port=") {
						portRange := strings.TrimPrefix(part, "client_port=")
						portNumbers := strings.Split(portRange, "-")
						if len(portNumbers) == 2 {
							clientPortStart, _ = strconv.Atoi(portNumbers[0])
							clientPortEnd, _ = strconv.Atoi(portNumbers[1])
						}
					}
				}

				// Check transport type (UDP)
				if strings.Contains(transportLine, "RTP/AVP") {
					if clientPortStart > 0 && clientPortEnd > 0 {
						transportType = "UDP"
						setupResponse := fmt.Sprintf("RTSP/1.0 200 OK\r\nCSeq: %d\r\nTransport: RTP/AVP;unicast;client_port=%d-%d\r\nSession: 12345678\r\n\r\n", cseq, clientPortStart, clientPortEnd)
						if _, err := conn.Write([]byte(setupResponse)); err != nil {
							fmt.Println("Error writing SETUP response:", err)
						}
						//go streamImageUDP(clientIP, clientPortStart) // Start RTP over UDP
						go videoStream(clientIP, clientPortStart)  // Start RTP over UDP
						go sendRTCPReport(clientIP, clientPortEnd) // Start RTCP over UDP
						continue
					}
				}
			}

			// If we reach here, it means we didn't find valid transport
			unsupportedResponse := fmt.Sprintf("RTSP/1.0 461 Unsupported Transport\r\nCSeq: %d\r\n\r\n", cseq)
			if _, err := conn.Write([]byte(unsupportedResponse)); err != nil {
				fmt.Println("Error writing unsupported transport response:", err)
			}
			continue
		}

		if strings.Contains(reqStr, "PLAY") {
			cseq++

			if strings.Contains(reqStr, "Session:") {
				playResponse := fmt.Sprintf("RTSP/1.0 200 OK\r\nCSeq: %d\r\nSession: 12345678\r\n\r\n", cseq)
				if _, err := conn.Write([]byte(playResponse)); err != nil {
					fmt.Println("Error writing PLAY response:", err)
				}
				if transportType == "UDP" {
					go streamImageUDP(clientIP, clientPortStart)
					go sendRTCPReport(clientIP, clientPortEnd)
				}
			} else {
				playErrorResponse := fmt.Sprintf("RTSP/1.0 454 Session Not Found\r\nCSeq: %d\r\n\r\n", cseq)
				if _, err := conn.Write([]byte(playErrorResponse)); err != nil {
					fmt.Println("Error writing PLAY error response:", err)
				}
			}
		}
		if strings.Contains(reqStr, "TEARDOWN") {
			cseq++

			// Optionally, extract session ID if necessary
			var sessionID string
			if strings.Contains(reqStr, "Session:") {
				sessionID = extractSessionID(reqStr) // You need to implement this function
			}

			// Here you would add logic to clean up any resources associated with the session
			// For example, stop the streaming, release ports, etc.

			teardownResponse := fmt.Sprintf("RTSP/1.0 200 OK\r\nCSeq: %d\r\nSession: %s\r\n\r\n", cseq, sessionID)
			if _, err := conn.Write([]byte(teardownResponse)); err != nil {
				fmt.Println("Error writing TEARDOWN response:", err)
			}

			// Optionally log that the stream was stopped
			fmt.Println("TEARDOWN received, session stopped:", sessionID)
			continue
		}
	}
}

func streamImageUDP(clientIP string, clientPort int) {
	imageData, err := os.ReadFile("image.jpg")
	if err != nil {
		fmt.Println("Error reading image file:", err)
		return
	}

	// Initialize the RTP stream parameters
	sequenceNumber := uint16(1)
	timestamp := uint32(0)

	// Setup UDP connection to client
	serverAddr := &net.UDPAddr{
		IP:   net.ParseIP(clientIP),
		Port: clientPort,
	}
	udpConn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		fmt.Println("Error dialing UDP:", err)
		return
	}
	defer func(udpConn *net.UDPConn) {
		err := udpConn.Close()
		if err != nil {

		}
	}(udpConn)

	// Create the RTP header for JPEG
	rtpHeader := make([]byte, 12)
	rtpHeader[0] = 0x80 // Version: 2
	rtpHeader[1] = 26   // Payload Type: 26 for JPEG

	for {
		rtpHeader[2] = byte(sequenceNumber >> 8) // Sequence Number
		rtpHeader[3] = byte(sequenceNumber & 0xFF)
		rtpHeader[4] = byte(timestamp >> 24) // Timestamp
		rtpHeader[5] = byte((timestamp >> 16) & 0xFF)
		rtpHeader[6] = byte((timestamp >> 8) & 0xFF)
		rtpHeader[7] = byte(timestamp & 0xFF)

		// Construct RTP packet
		rtpPacket := append(rtpHeader, imageData...)
		_, err := udpConn.Write(rtpPacket)
		if err != nil {
			fmt.Println("Error writing RTP packet:", err)
			return
		}

		// Increment sequence number and timestamp
		sequenceNumber++
		timestamp += 90000                // Increment timestamp by frame rate (30fps = 90000)
		time.Sleep(33 * time.Millisecond) // Send 30 fps
	}
}

// Send RTCP Report
func sendRTCPReport(clientIP string, clientPort int) {
	rtcpPacket := []byte{ // Minimal RTCP report (Receiver Report)
		0x80, 0xC9, 0x00, 0x07, // Version, Padding, Receiver Report (RR), Length
		0x00, 0x00, 0x00, 0x00, // SSRC (Sender Source Identifier)
		0x00, 0x00, 0x00, 0x00, // Fraction Lost and Cumulative Number of Packets Lost
		0x00, 0x00, 0x00, 0x00, // Extended Highest Sequence Number Received
		0x00, 0x00, 0x00, 0x00, // Interarrival Jitter
		0x00, 0x00, 0x00, 0x00, // Last SR Timestamp
		0x00, 0x00, 0x00, 0x00, // Delay since Last SR
	}

	serverAddr := &net.UDPAddr{
		IP:   net.ParseIP(clientIP),
		Port: clientPort,
	}
	udpConn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		fmt.Println("Error dialing UDP for RTCP:", err)
		return
	}
	defer func(udpConn *net.UDPConn) {
		err := udpConn.Close()
		if err != nil {

		}
	}(udpConn)

	for {
		_, _ = udpConn.Write(rtcpPacket)
		time.Sleep(5 * time.Second) // Send RTCP report every 5 seconds
	}
}
func videoStream(clientAddr string, clientPort int) {

	// Set up UDP connection for RTP
	raddr := &net.UDPAddr{
		IP:   net.ParseIP(clientAddr),
		Port: clientPort,
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		fmt.Println("Error setting up UDP connection:", err)
		return
	}
	defer conn.Close()

	// Import video
	file, err := os.Open("test_video.mp4")
	if err != nil {
		fmt.Println("Error opening video file:", err)
		return
	}
	defer file.Close()

	ticker := time.NewTicker(33 * time.Millisecond) // 30 fps
	defer ticker.Stop()
	// Basic RTP header
	rtpHeader := make([]byte, 12)
	rtpHeader[0] = 0x80         // RTP version 2
	rtpHeader[1] = 96           // Payload type for dynamic
	sequenceNumber := uint16(1) // Start sequence number
	timestamp := uint32(0)      // Timestamp, updated each frame

	// Read video file and send over RTP
	for range ticker.C {
		frame, err := extractH264(file)
		if err == io.EOF {
			fmt.Println("End of video file reached")
			break
		}
		if err != nil {
			fmt.Println("Error extracting frame:", err)
			break
		}
		// Set RTP sequence number and timestamp
		rtpHeader[2] = byte(sequenceNumber >> 8)
		rtpHeader[3] = byte(sequenceNumber & 0xFF)
		rtpHeader[4] = byte(timestamp >> 24)
		rtpHeader[5] = byte((timestamp >> 16) & 0xFF)
		rtpHeader[6] = byte((timestamp >> 8) & 0xFF)
		rtpHeader[7] = byte(timestamp & 0xFF)
		// Send the RTP packet (header + frame data)
		rtpPacket := append(rtpHeader, frame...)
		_, err = conn.Write(rtpPacket)
		if err != nil {
			fmt.Println("Error sending RTP packet:", err)
			break
		}

		// Update sequence and timestamp for next frame
		sequenceNumber++
		timestamp += 3000 // Adjust timestamp increment based on frame rate

	}
}

func extractH264(file *os.File) ([]byte, error) {
	buffer := make([]byte, 1024)
	n, err := file.Read(buffer)
	if err != nil {
		fmt.Println("Error reading file:", err)
		return nil, err
	}
	return buffer[:n], nil
}

func extractCSeq(reqStr string) int {
	lines := strings.Split(reqStr, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "CSeq:") {
			cseqStr := strings.TrimSpace(strings.TrimPrefix(line, "CSeq:"))
			cseq, _ := strconv.Atoi(cseqStr)
			return cseq
		}
	}
	return 0
}
func extractSessionID(req string) string {
	lines := strings.Split(req, "\r\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Session:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Session:"))
		}
	}
	return ""
}

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
