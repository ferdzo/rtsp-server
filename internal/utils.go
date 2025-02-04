package internal

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

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

func extractH264(file *os.File) ([]byte, error) {
	buffer := make([]byte, 1024)
	n, err := file.Read(buffer)
	if err != nil {
		fmt.Println("Error reading file:", err)
		return nil, err
	}
	return buffer[:n], nil
}

func parseTransportLine(reqStr string) (string, int, int, error) {
	transportLine := strings.Split(reqStr, "Transport:")[1]
	transportLine = strings.TrimSpace(transportLine)

	var clientPortStart, clientPortEnd int
	for _, part := range strings.Split(transportLine, ";") {
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

	if clientPortStart > 0 && clientPortEnd > 0 && strings.Contains(transportLine, "RTP/AVP") {
		return "UDP", clientPortStart, clientPortEnd, nil
	}
	return "", 0, 0, fmt.Errorf("unsupported transport")
}

func fetchServerIP(conn net.Conn) string {
	localAddr := conn.LocalAddr().String()
	return localAddr
}
