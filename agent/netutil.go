package main

import (
	"net"
)

func getInterfaceAddrs() ([]string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var addrs []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		ipAddrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range ipAddrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			if ipNet.IP.To4() != nil && !ipNet.IP.IsLoopback() {
				addrs = append(addrs, ipNet.IP.String())
			}
		}
	}
	return addrs, nil
}
