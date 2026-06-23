package gosysinfo

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	netBase           = "/sys/class/net"
	procNetIfInet6    = "/proc/net/if_inet6"
	procNetFibTrie    = "/proc/net/fib_trie"
	procNetRoute      = "/proc/net/route"
)

type NetworkConnectionInfo struct {
	MACAddress string
}

type NetworkIPInfo struct {
	IPv4 []string
	IPv6 []string
}

type NetworkStatistics struct {
	Addr_assign_type     string
	Address              string
	Addr_len             string
	Broadcast            string
	Carrier              string
	Carrier_changes      string
	Carrier_down_count   string
	Carrier_up_count     string
	Device               string
	Dev_id               string
	Dev_port             string
	Dormant              string
	Duplex               string
	Flags                string
	Gro_flush_timeout    string
	Ifalias              string
	Ifindex              string
	Iflink               string
	Link_mode            string
	Mtu                  string
	Name_assign_type     string
	Napi_defer_hard_irqs string
	Netdev_group         string
	Operstate            string
	Phy80211             string
	Power                string
	Proto_down           string
	Queues               string
	Speed                string
	Statistics           string
	Subsystem            string
	Testing              string
	Threaded             string
	Tx_queue_len         string
	Type                 string
	Uevent               string
	Wireless             string
}

type Network interface {
	ListNetworkInterfaces() ([]string, error)
	GetNetworkConnectionInfo(ifname string) (*NetworkConnectionInfo, error)
	GetNetworkIPInfo(ifname string) (*NetworkIPInfo, error)
	GetNetworkStatistics(ifname string) (*NetworkStatistics, error)
}

var _ Network = Reader{}

func ListNetworkInterfaces() ([]string, error) {
	return listSysfsClassEntries(netBase)
}

func GetNetworkConnectionInfo(r SysReader, ifname string) (*NetworkConnectionInfo, error) {
	path, err := checkNetworkPathByName(ifname)
	if err != nil {
		return nil, err
	}

	return &NetworkConnectionInfo{
		MACAddress: readSysFile(r, filepath.Join(path, "address")),
	}, nil
}

func GetNetworkIPInfo(r SysReader, ifname string) (*NetworkIPInfo, error) {
	if _, err := checkNetworkPathByName(ifname); err != nil {
		return nil, err
	}

	ipv6 := parseIPv6ForInterface(readSysFile(r, procNetIfInet6), ifname)
	ipv4 := parseIPv4ForInterface(
		readSysFile(r, procNetFibTrie),
		readSysFile(r, procNetRoute),
		ifname,
	)

	return &NetworkIPInfo{
		IPv4: ipv4,
		IPv6: ipv6,
	}, nil
}

func GetNetworkStatistics(r SysReader, ifname string) (*NetworkStatistics, error) {
	path, err := checkNetworkPathByName(ifname)
	if err != nil {
		return nil, err
	}

	read := func(field string) string {
		return readSysFile(r, filepath.Join(path, field))
	}

	return &NetworkStatistics{
		Addr_assign_type:     read("addr_assign_type"),
		Address:              read("address"),
		Addr_len:             read("addr_len"),
		Broadcast:            read("broadcast"),
		Carrier:              read("carrier"),
		Carrier_changes:      read("carrier_changes"),
		Carrier_down_count:   read("carrier_down_count"),
		Carrier_up_count:     read("carrier_up_count"),
		Device:               "",
		Dev_id:               read("dev_id"),
		Dev_port:             read("dev_port"),
		Dormant:              read("dormant"),
		Duplex:               read("duplex"),
		Flags:                read("flags"),
		Gro_flush_timeout:    read("gro_flush_timeout"),
		Ifalias:              read("ifalias"),
		Ifindex:              read("ifindex"),
		Iflink:               read("iflink"),
		Link_mode:            read("link_mode"),
		Mtu:                  read("mtu"),
		Name_assign_type:     read("name_assign_type"),
		Napi_defer_hard_irqs: read("napi_defer_hard_irqs"),
		Netdev_group:         read("group"),
		Operstate:            read("operstate"),
		Phy80211:             read("phy80211"),
		Power:                "",
		Proto_down:           read("proto_down"),
		Queues:               "",
		Speed:                read("speed"),
		Statistics:           readSysFile(r, filepath.Join(path, "statistics", "rx_bytes")),
		Subsystem:            "",
		Testing:              read("testing"),
		Threaded:             read("threaded"),
		Tx_queue_len:         read("tx_queue_len"),
		Type:                 read("type"),
		Uevent:               read("uevent"),
		Wireless:             "",
	}, nil
}

func (Reader) ListNetworkInterfaces() ([]string, error) {
	return ListNetworkInterfaces()
}

func (r Reader) GetNetworkConnectionInfo(ifname string) (*NetworkConnectionInfo, error) {
	return GetNetworkConnectionInfo(r, ifname)
}

func (r Reader) GetNetworkIPInfo(ifname string) (*NetworkIPInfo, error) {
	return GetNetworkIPInfo(r, ifname)
}

func (r Reader) GetNetworkStatistics(ifname string) (*NetworkStatistics, error) {
	return GetNetworkStatistics(r, ifname)
}

func checkNetworkPathByName(ifname string) (string, error) {
	return checkNetworkPathByNameFn(ifname)
}

var checkNetworkPathByNameFn = func(ifname string) (string, error) {
	path := filepath.Join(netBase, ifname)
	if _, err := os.Stat(path); err != nil {
		return "", ErrNetworkNotFound
	}
	return path, nil
}

func parseIPv6ForInterface(content, ifname string) []string {
	if content == "" {
		return nil
	}

	var addrs []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 6 || fields[len(fields)-1] != ifname {
			continue
		}
		addrs = append(addrs, formatProcIPv6(fields[0]))
	}
	return addrs
}

func formatProcIPv6(hexAddr string) string {
	if len(hexAddr) != 32 {
		return hexAddr
	}

	parts := make([]string, 8)
	for i := range 8 {
		parts[i] = hexAddr[i*4 : i*4+4]
	}
	ip := net.ParseIP(strings.Join(parts, ":"))
	if ip == nil {
		return hexAddr
	}
	return ip.String()
}

func parseIPv4ForInterface(fibTrie, routeTable, ifname string) []string {
	localAddrs := parseLocalIPv4FromFibTrie(fibTrie)
	networks := parseInterfaceNetworksFromRoute(routeTable, ifname)

	var matched []string
	for _, addr := range localAddrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			continue
		}
		for _, network := range networks {
			if network.Contains(ip) {
				matched = append(matched, addr)
				break
			}
		}
	}
	return matched
}

func parseLocalIPv4FromFibTrie(content string) []string {
	if content == "" {
		return nil
	}

	inLocal := false
	var pending string
	var addrs []string

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "Local:" {
			inLocal = true
			continue
		}
		if !inLocal {
			continue
		}
		if strings.HasSuffix(trimmed, ":") && trimmed != "Local:" {
			break
		}

		if strings.HasPrefix(trimmed, "|-- ") {
			pending = strings.TrimPrefix(trimmed, "|-- ")
			continue
		}
		if pending != "" && strings.Contains(trimmed, "host LOCAL") {
			if net.ParseIP(pending) != nil {
				addrs = append(addrs, pending)
			}
			pending = ""
		}
	}
	return addrs
}

func parseInterfaceNetworksFromRoute(content, ifname string) []*net.IPNet {
	if content == "" {
		return nil
	}

	lines := strings.Split(content, "\n")
	if len(lines) < 2 {
		return nil
	}

	var networks []*net.IPNet
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 8 || fields[0] != ifname {
			continue
		}

		flags, err := strconv.ParseInt(fields[3], 16, 64)
		if err != nil || flags&0x1 == 0 {
			continue
		}

		dest := parseRouteHexIP(fields[1])
		mask := parseRouteHexIP(fields[7])
		if dest == "" || mask == "" {
			continue
		}

		ip := net.ParseIP(dest)
		if ip == nil {
			continue
		}
		ip = ip.To4()
		if ip == nil {
			continue
		}

		maskIP := net.ParseIP(mask).To4()
		if maskIP == nil {
			continue
		}
		maskBytes := net.IPMask(maskIP)

		networks = append(networks, &net.IPNet{IP: ip.Mask(maskBytes), Mask: maskBytes})
	}
	return networks
}

func parseRouteHexIP(hex string) string {
	if len(hex) != 8 {
		return ""
	}
	a, err1 := strconv.ParseUint(hex[6:8], 16, 8)
	b, err2 := strconv.ParseUint(hex[4:6], 16, 8)
	c, err3 := strconv.ParseUint(hex[2:4], 16, 8)
	d, err4 := strconv.ParseUint(hex[0:2], 16, 8)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d", a, b, c, d)
}
