package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

func handleRTSP(conn net.Conn) {
	defer func() {
		if err := conn.Close(); err != nil {
			fmt.Println("Error closing connection:", err)
		}
	}()

	cseq := 0 // Initialize CSeq

	reader := bufio.NewReader(conn)
	for {
		var request strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
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

		// Handle CSeq value extraction
		if strings.Contains(reqStr, "CSeq:") {
			// Extract CSeq value
			cseqIndex := strings.Index(reqStr, "CSeq:")
			if cseqIndex != -1 {
				cseqValue := reqStr[cseqIndex+5 : strings.Index(reqStr[cseqIndex:], "\n")+cseqIndex]
				if value, err := strconv.Atoi(strings.TrimSpace(cseqValue)); err == nil {
					cseq = value // Update CSeq from the client
				} else {
					fmt.Println("Invalid CSeq:", cseqValue)
				}
			}
		}

		// Handle OPTIONS request
		if strings.Contains(reqStr, "OPTIONS") {
			cseq++
			optionsResponse := fmt.Sprintf("RTSP/1.0 200 OK\r\nCSeq: %d\r\nPublic: OPTIONS, DESCRIBE, SETUP, TEARDOWN, PLAY, PAUSE\r\n\r\n", cseq)
			if _, err := conn.Write([]byte(optionsResponse)); err != nil {
				fmt.Println("Error writing OPTIONS response:", err)
			} else {
				fmt.Println("Sent OPTIONS response:", optionsResponse)
			}
			continue // Continue to next request
		}

		// Handle DESCRIBE request
		if strings.Contains(reqStr, "DESCRIBE") {
			cseq++
			describeResponse := fmt.Sprintf(`RTSP/1.0 200 OK
CSeq: %d
Content-Base: rtsp://127.0.0.1:554/
Content-Type: application/sdp
Content-Length: 135

v=0
o=- 0 0 IN IP4 127.0.0.1
s=JPEG Stream
c=IN IP4 127.0.0.1
t=0 0
a=tool:golang-rtsp-server
m=video 5000 RTP/AVP 26
a=rtpmap:26 JPEG/90000
`, cseq)

			if _, err := conn.Write([]byte(describeResponse)); err != nil {
				fmt.Println("Error writing DESCRIBE response:", err)
			} else {
				fmt.Println("Sent DESCRIBE response:", describeResponse)
			}
			continue
		}

		// Handle SETUP request
		if strings.Contains(reqStr, "SETUP") {
			if strings.Contains(reqStr, "RTP/AVP/TCP") {
				cseq++
				setupResponse := fmt.Sprintf("RTSP/1.0 200 OK\r\nCSeq: %d\r\nTransport: RTP/AVP/TCP;unicast;interleaved=0-1\r\nSession: 12345678\r\n\r\n", cseq)
				if _, err := conn.Write([]byte(setupResponse)); err != nil {
					fmt.Println("Error writing SETUP response:", err)
				} else {
					fmt.Println("Sent SETUP response:", setupResponse)
				}
				go streamImage(conn) // Start streaming images
				continue
			} else {
				cseq++
				unsupportedResponse := fmt.Sprintf("RTSP/1.0 461 Unsupported Transport\r\nCSeq: %d\r\n\r\n", cseq)
				if _, err := conn.Write([]byte(unsupportedResponse)); err != nil {
					fmt.Println("Error writing unsupported transport response:", err)
				} else {
					fmt.Println("Sent unsupported transport response:", unsupportedResponse)
				}
				continue
			}
		} else if strings.Contains(reqStr, "PLAY") {
			fmt.Println("Received PLAY request") // This should now show up
			cseq++

			// Check if Session header is present
			if strings.Contains(reqStr, "Session:") {
				playResponse := fmt.Sprintf("RTSP/1.0 200 OK\r\nCSeq: %d\r\nSession: 12345678\r\n\r\n", cseq)
				if _, err := conn.Write([]byte(playResponse)); err != nil {
					fmt.Println("Error writing PLAY response:", err)
				} else {
					fmt.Println("Sent PLAY response:", playResponse)
				}

				// Start sending RTP packets
				go streamImage(conn)
			} else {
				// If Session is not found, respond with error
				playErrorResponse := fmt.Sprintf("RTSP/1.0 454 Session Not Found\r\nCSeq: %d\r\n\r\n", cseq)
				if _, err := conn.Write([]byte(playErrorResponse)); err != nil {
					fmt.Println("Error writing PLAY error response:", err)
				}
			}
		}
	}
}

func main() {
	listener, err := net.Listen("tcp", ":554")
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}
	defer listener.Close()
	fmt.Println("Listening on port 554")

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("Error: %s\n", err)
			continue
		}
		go handleRTSP(conn)
	}
}

func streamImage(conn net.Conn) {
	imageData, err := os.ReadFile("image.jpg")
	if err != nil {
		fmt.Println("Error reading image file:", err)
		return
	}

	// Initialize RTP sequence number and timestamp
	sequenceNumber := uint16(1)
	timestamp := uint32(0)

	// Define RTP header fields
	rtpHeader := make([]byte, 12) // 12-byte RTP header
	rtpHeader[0] = 0x80           // Version 2, no padding, no extension, 1 contributing source
	rtpHeader[1] = 26             // Payload type 26 for JPEG

	// Loop to send the JPEG frames in RTP packets
	for {
		// Set the RTP header sequence number and timestamp
		rtpHeader[2] = byte(sequenceNumber >> 8)   // Sequence number (high byte)
		rtpHeader[3] = byte(sequenceNumber & 0xFF) // Sequence number (low byte)
		rtpHeader[4] = byte(timestamp >> 24)       // Timestamp (high byte)
		rtpHeader[5] = byte((timestamp >> 16) & 0xFF)
		rtpHeader[6] = byte((timestamp >> 8) & 0xFF)
		rtpHeader[7] = byte(timestamp & 0xFF)

		// SSRC (arbitrary value, remains constant)
		rtpHeader[8] = 0x00
		rtpHeader[9] = 0x00
		rtpHeader[10] = 0x00
		rtpHeader[11] = 0x01

		// The JPEG header is required for RTP (RFC 2435)
		jpegHeader := make([]byte, 8)
		jpegHeader[0] = 0x00 // Type-specific
		jpegHeader[1] = 0x00 // Fragment offset high byte (for larger JPEGs, could be used for fragmentation)
		jpegHeader[2] = 0x00 // Fragment offset mid byte
		jpegHeader[3] = 0x00 // Fragment offset low byte
		jpegHeader[4] = 0x00 // JPEG type (could be 0 or any other value for JPEG)
		jpegHeader[5] = 0x01 // Quality factor (1-100)
		jpegHeader[6] = 0x00 // Width in 8-pixel blocks
		jpegHeader[7] = 0x00 // Height in 8-pixel blocks

		// Combine RTP header, JPEG header, and image data
		rtpPacket := append(rtpHeader, jpegHeader...)
		rtpPacket = append(rtpPacket, imageData...)

		// Send the RTP packet
		if _, err := conn.Write(rtpPacket); err != nil {
			fmt.Printf("Error writing RTP packet: %s\n", err)
			return
		}

		fmt.Printf("Sent RTP packet with sequence number: %d\n", sequenceNumber)

		// Increment the sequence number and timestamp for the next packet
		sequenceNumber++
		timestamp += 90000 // Increment timestamp based on 90kHz clock (1 second for now)

		time.Sleep(1 * time.Second) // Simulate frame rate (adjust for actual frame rate)
	}
}
