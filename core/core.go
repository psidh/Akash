package core

import (
	"hash/fnv"
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type UserConfig struct {
	Host            string    `json:"host"`
	Port            string    `json:"listen"`
	Backends        []Backend `json:"Backends"`
	Algorithm       string    `json:"algorithm"`
	MaxConnections  int       `json:"max_connections"`
	TimeoutSeconds  int       `json:"timeout_seconds"`
	HealthCheckPath string    `json:"health_check_path"`
	HealthCheckPort string    `json:"health_check_port"`
	HealthCheckFreq int       `json:"health_check_freq"`
}

type Backend struct {
	Address           string `json:"address"`
	Weight            int    `json:"weight"`
	IsHealthy         bool   `json:"-"`
	ActiveConnections int32  `json:"-"`
	mutex             sync.Mutex
	LastChecked       time.Time `json:"-"`
	CurrentWeight     int       `json:"-"`
}

type Algorithm int

const (
	RoundRobin Algorithm = iota
	LeastConnections
	IPHash
	WeightedRoundRobin
)

type LoadBalancer struct {
	Config          *UserConfig
	Backends        []*Backend
	Algo            Algorithm
	ConnectionCount int32
	Index           int32
	BackendCounts   []int32
	BackendFails    []int32
}

func ParseAlgorithm(name string) Algorithm {
	switch strings.ToLower(name) {
	case "round_robin":
		return RoundRobin
	case "least_conn":
		return LeastConnections
	case "ip_hash":
		return IPHash
	case "w_round_robin":
		return WeightedRoundRobin
	default:
		log.Printf("Unknown algorithm %s, defaulting to round robin", name)
		return RoundRobin
	}
}

func (lb *LoadBalancer) GetNextBackend(clientAddress string) (*Backend, int, func()) {
	var idx int
	var backend *Backend

	switch lb.Algo {
	case RoundRobin:

		for attempts := 0; attempts < len(lb.Backends); attempts++ {
			idx := int(atomic.AddInt32(&lb.Index, 1)) % len(lb.Backends)

			candidate := lb.Backends[idx]

			candidate.mutex.Lock()
			healthy := candidate.IsHealthy
			candidate.mutex.Unlock()
			if healthy {
				backend = candidate
				break
			}
		}

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

	case WeightedRoundRobin:
		var total int
		var selected *Backend
		var selectedIdx int

		for _, b := range lb.Backends {
			total += b.Weight
		}

		maxWeight := -1

		for i, b := range lb.Backends {
			b.mutex.Lock()
			if !b.IsHealthy {
				b.mutex.Unlock()
				continue
			}

			b.CurrentWeight += b.Weight

			if b.CurrentWeight > maxWeight {
				maxWeight = b.CurrentWeight
				selected = b
				selectedIdx = i
			}
			b.mutex.Unlock()
		}

		if selected != nil {
			selected.mutex.Lock()
			selected.CurrentWeight -= total
			selected.mutex.Unlock()
			backend = selected
			idx = selectedIdx
		}

	default:
		for attempts := 0; attempts < len(lb.Backends); attempts++ {
			idx := int(atomic.AddInt32(&lb.Index, 1)) % len(lb.Backends)

			candidate := lb.Backends[idx]

			candidate.mutex.Lock()
			healthy := candidate.IsHealthy
			candidate.mutex.Unlock()
			if healthy {
				backend = candidate
				break
			}
		}
	}

	atomic.AddInt32(&lb.ConnectionCount, 1)
	atomic.AddInt32(&lb.BackendCounts[idx], 1)

	release := func() {
		backend.mutex.Lock()
		backend.ActiveConnections--
		backend.mutex.Unlock()

		atomic.AddInt32(&lb.ConnectionCount, -1)
	}

	if backend == nil {
		return nil, -1, func() {}
	}

	return backend, idx, release
}
