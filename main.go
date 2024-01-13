package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"time"
)

// Session is the UDP session info
type Session struct {
	clientAddr *net.UDPAddr
	serverConn *net.UDPConn
	timeout    time.Time
}

// Forwarder is the info of forwarder
type Forwarder struct {
	fromAddr  *net.UDPAddr
	toAddr    *net.UDPAddr
	localConn *net.UDPConn
	sessions  map[string]*Session
	mu        sync.Mutex
	timeout   time.Duration
}

func main() {
	// Parse command line arguments
	fromAddrStr := flag.String("from", "127.0.0.1:8080", "UDP address to forward from")
	toAddrStr := flag.String("to", "127.0.0.1:9090", "UDP address to forward to")
	timeout := flag.Duration("timeout", 60*time.Second, "Timeout for inactive sessions")
	flag.Parse()

	fromAddr, err := net.ResolveUDPAddr("udp", *fromAddrStr)
	if err != nil {
		log.Fatal("Error resolving UDP address:", err)
	}

	toAddr, err := net.ResolveUDPAddr("udp", *toAddrStr)
	if err != nil {
		log.Fatal("Error resolving UDP address:", err)
	}

	// Create and start the UDP forwarder with a timeout
	forwarder := NewForwarder(fromAddr, toAddr, *timeout)
	go forwarder.Start()

	// Wait for a termination signal
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	<-signalChan

	// Stop the forwarder on termination signal
	forwarder.Stop()
	log.Println("UDP forwarder stopped")
}

// NewForwarder creates a new UDP forwarder with a timeout feature
func NewForwarder(fromAddr, toAddr *net.UDPAddr, timeout time.Duration) *Forwarder {
	localConn, err := net.ListenUDP("udp", fromAddr)
	if err != nil {
		log.Fatal("Error listening on UDP address:", err)
	}

	return &Forwarder{
		fromAddr:  fromAddr,
		toAddr:    toAddr,
		localConn: localConn,
		sessions:  make(map[string]*Session),
		timeout:   timeout,
	}
}

// Start starts the UDP forwarder with a timeout feature
func (f *Forwarder) Start() {
	log.Printf("UDP forwarder started - From: %v, To: %v, Timeout: %v\n", f.fromAddr, f.toAddr, f.timeout)

	buffer := make([]byte, 1500)

	// Periodically clean up inactive sessions
	go func() {
		for range time.Tick(f.timeout / 2) {
			f.cleanupInactiveSessions()
		}
	}()

	for {
		n, clientAddr, err := f.localConn.ReadFromUDP(buffer)
		if err != nil {
			log.Println("Error reading from UDP:", err)
			continue
		}

		clientKey := fmt.Sprintf("%s:%d", clientAddr.IP.String(), clientAddr.Port)

		f.mu.Lock()
		session, ok := f.sessions[clientKey]
		if !ok {
			serverConn, err := net.DialUDP("udp", nil, f.toAddr)
			if err != nil {
				log.Println("Error connecting to server:", err)
				f.mu.Unlock()
				continue
			}

			session = &Session{
				clientAddr: clientAddr,
				serverConn: serverConn,
				timeout:    time.Now().Add(f.timeout),
			}

			f.sessions[clientKey] = session

			// Log new connection
			log.Printf("New session established with client: %s\n", clientKey)

			go f.handleSession(clientKey, session)
		} else {
			// Update session timeout for active sessions
			session.timeout = time.Now().Add(f.timeout)
		}
		f.mu.Unlock()

		// Forward the UDP packet to the server
		_, err = session.serverConn.Write(buffer[:n])
		if err != nil {
			log.Println("Error forwarding UDP packet:", err)
		}
	}
}

// cleanupInactiveSessions closes inactive sessions
func (f *Forwarder) cleanupInactiveSessions() {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now()
	for key, session := range f.sessions {
		if session.timeout.Before(now) {
			log.Printf("Cleaning up inactive session %s\n", key)
			session.serverConn.Close()
			delete(f.sessions, key)
		}
	}
}

// handleSession handles a UDP session
func (f *Forwarder) handleSession(clientKey string, session *Session) {
	buffer := make([]byte, 1500)

	for {
		n, _, err := session.serverConn.ReadFromUDP(buffer)
		if err != nil {
			log.Printf("Session %s closed\n", clientKey)

			f.mu.Lock()
			delete(f.sessions, clientKey)
			f.mu.Unlock()

			session.serverConn.Close()
			return
		}

		// Forward the UDP packet back to the client
		_, err = f.localConn.WriteToUDP(buffer[:n], session.clientAddr)
		if err != nil {
			log.Println("Error forwarding UDP packet to client:", err)
		}
	}
}

// Stop stops the UDP forwarder
func (f *Forwarder) Stop() {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, session := range f.sessions {
		session.serverConn.Close()
	}

	f.localConn.Close()
}
