package utils

import (
	"net"
)

const PORT = "1234"

func GetIpAddress() string {
	var result string = ""
	ip, err := net.InterfaceAddrs()
	if err != nil {
		return result
	}
	for _, address := range ip {
		if ipNet, ok := address.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				result = ipNet.IP.String()
			}
		}
	}
	return result
}

func GenerateNodeId() string {
	ip := GetIpAddress()
	return ip + ":" + PORT
}
