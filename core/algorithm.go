package types

import (
	"hash/fnv"
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
)

type UserConfig struct {
	Host           string   `json:"host"`
	Port           string   `json:"listen"`
	Backends       []string `json:"Backends"`
	Algorithm      string   `json:"algorithm"`
	MaxConnections int      `json:"max_connections"`
	TimeoutSeconds int      `json:"timeout_seconds"`
}

type Backend struct {
	Address           string
	IsHealthy         bool
	ActiveConnections int32
	mutex             sync.Mutex
}

type Algorithm int

const (
	RoundRobin Algorithm = iota
	LeastConnections
	IPHash
)

type LoadBalancer struct {
	Config          *UserConfig
	Backends        []*Backend
	Algo            Algorithm
	ConnectionCount int32
	Index           int32
	BackendCounts   []int32
}


func ParseAlgorithm(name string) Algorithm {
	switch strings.ToLower(name) {
	case "round_robin":
		return RoundRobin
	case "least_conn":
		return LeastConnections
	case "ip_hash":
		return IPHash
	default:
		log.Printf("Unknown algorithm %s, defaulting to round robin", name)
		return RoundRobin
	}
}


func (lb *LoadBalancer) GetNextBackend(clientAddress string) (*Backend, func()) {
	var idx int
	var backend *Backend

	switch lb.Algo {
	case RoundRobin:
		idx = int(atomic.AddInt32(&lb.Index, 1)) % len(lb.Backends)
		backend = lb.Backends[idx]

	case LeastConnections:
		var minIdx int
		var minConn int32

		lb.Backends[0].mutex.Lock()
		minConn = lb.Backends[0].ActiveConnections
		lb.Backends[0].mutex.Unlock()

		for i := 1; i < len(lb.Backends); i++ {
			lb.Backends[i].mutex.Lock()
			currConn := lb.Backends[i].ActiveConnections
			lb.Backends[i].mutex.Unlock()

			if currConn < minConn {
				minConn = currConn
				minIdx = i
			}
		}

		backend = lb.Backends[minIdx]

		backend.mutex.Lock()
		backend.ActiveConnections++
		backend.mutex.Unlock()

		idx = minIdx

	case IPHash:
		host, _, err := net.SplitHostPort(clientAddress)
		if err != nil {
			host = clientAddress
		}
		h := fnv.New32a()
		h.Write([]byte(host))
		hashVal := h.Sum32()

		idx = int(hashVal) % len(lb.Backends)
		backend = lb.Backends[idx]

	default:
		idx = int(atomic.AddInt32(&lb.Index, 1)) % len(lb.Backends)
		backend = lb.Backends[idx]
	}

	atomic.AddInt32(&lb.ConnectionCount, 1)

	release := func() {
		backend.mutex.Lock()
		backend.ActiveConnections--
		backend.mutex.Unlock()

		atomic.AddInt32(&lb.ConnectionCount, -1)
	}

	return backend, release
}
