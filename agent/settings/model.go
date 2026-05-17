package settings

import (
	"net"
)

type ServerSettings struct {
	ServerAddr       net.IP
	ServerPort       int
	ServerDomainName string
}

type AgentSettings struct {
	SyncFrequency int
}

type Settings struct {
	Server ServerSettings
	Agent  AgentSettings
}
