package internal

import (
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

// Streaming image over UDP
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

// Streaming H264 video over UDP
func videoStreamUDP(clientAddr string, clientPort int) {

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
	defer func(conn *net.UDPConn) {
		err := conn.Close()
		if err != nil {

		}
	}(conn)

	// Import video
	file, err := os.Open("test_video.mp4")
	if err != nil {
		fmt.Println("Error opening video file:", err)
		return
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {

		}
	}(file)

	ticker := time.NewTicker(33 * time.Millisecond) // 30 fps
	defer ticker.Stop()
	// Channel to hold frames
	frameChan := make(chan []byte)

	// Goroutine to read frames from video file
	go func() {
		for {
			frame, err := extractH264(file)
			if err == io.EOF {
				fmt.Println("End of video file reached")
				close(frameChan) // Signal completion
				return
			}
			if err != nil {
				fmt.Println("Error extracting frame:", err)
				close(frameChan)
				return
			}
			frameChan <- frame // Send frame to channel
		}
	}()

	// Basic RTP header
	rtpHeader := make([]byte, 12)
	rtpHeader[0] = 0x80         // RTP version 2
	rtpHeader[1] = 96           // Payload type for dynamic
	sequenceNumber := uint16(1) // Start sequence number
	timestamp := uint32(0)      // Timestamp, updated each frame

	// Read video file and send over RTP
	for range ticker.C {
		select {
		case frame, ok := <-frameChan:
			if !ok {
				return
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
		default:
			continue
		}
	}
}
