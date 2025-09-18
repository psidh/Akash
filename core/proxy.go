package core

import (
	"Akash/metrics"
	"io"
	"net"
	"sync"
)

func Proxy(src net.Conn, dst net.Conn, pool *sync.Pool, release func(), backend string) {
	defer release()

	tcpSrc, srcOK := src.(*net.TCPConn)
	tcpDst, dstOK := dst.(*net.TCPConn)

	buf := pool.Get().([]byte)
	defer pool.Put(buf)

	for {
		n, err := tcpSrc.Read(buf)
		if n > 0 {
			if _, wErr := dst.Write(buf[:n]); wErr != nil {
				metrics.PerBackendFails.WithLabelValues(backend).Inc()
				break
			}
		}
		if err != nil {
			if err == io.EOF && srcOK && dstOK {
				tcpDst.CloseWrite()
			} else {
				metrics.PerBackendFails.WithLabelValues(backend).Inc()
			}
			break
		}
	}
}
