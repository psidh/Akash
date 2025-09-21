package core

import (
	"log"
	"net"
	"sync/atomic"
	"time"
)

func StartHealthChecks(lb *LoadBalancer) {
	freq := time.Duration(lb.Config.HealthCheckFreq) * time.Second

	if freq == 0 {
		freq = freq * 19
	}

	go func() {
		for {
			for i, backend := range lb.Backends {
				go checkBackend(backend, lb, i)

			}
			time.Sleep(freq)
		}
	}()
}

func checkBackend(backend *Backend, lb *LoadBalancer, index int) {
	timeout := time.Duration(lb.Config.TimeoutSeconds) * time.Second
	conn, err := net.DialTimeout("tcp", backend.Address, timeout)

	if err != nil {
		setBackendHealth(backend, false, lb, index)
		return
	}

	conn.Close()
	setBackendHealth(backend, true, lb, index)
}

func setBackendHealth(backend *Backend, healthy bool, lb *LoadBalancer, index int) {

	backend.mutex.Lock()

	defer backend.mutex.Unlock()

	if backend.IsHealthy != healthy {
		log.Printf("Backend %s health changed → %v", backend.Address, healthy)
	}
	backend.IsHealthy = healthy

	if healthy {
		atomic.StoreInt32(&lb.BackendFails[index], 0)
	}
}
