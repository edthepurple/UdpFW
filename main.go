package main

import (
    "flag"
    "fmt"
    "log"
    "net"
    "os"
    "os/signal"
    "strings"
    "sync"
    "time"
)

var fw = flag.String("forward", "127.0.0.1:2000~127.0.0.1:3000", "can be multiple: from~to")
var verboseLogging = flag.Bool("verbose", false, "verbose logging")
var timeoutDuration = flag.Duration("timeout", 60*time.Second, "timeout for inactive sessions")

// Session is the UDP session info
type Session struct {
    clientAddr    *net.UDPAddr
    serverConn    *net.UDPConn
    lastActiveTime time.Time
}

// Forwarder is the info of forward
type Forwarder struct {
    fromAddr  *net.UDPAddr
    toAddr    *net.UDPAddr
    localConn *net.UDPConn
    sessions  map[string]*Session
    mutex     sync.Mutex // Mutex to control access to 'sessions'
}

func verbosePrintf(format string, v ...interface{}) {
    if *verboseLogging {
        log.Printf(format, v...)
    }
}

func (f *Forwarder) addSession(key string, session *Session) {
    f.mutex.Lock()
    defer f.mutex.Unlock()
    f.sessions[key] = session
}

func (f *Forwarder) deleteSession(key string) {
    f.mutex.Lock()
    defer f.mutex.Unlock()
    delete(f.sessions, key)
}

func xorEncryptDecrypt(data []byte, key byte) {
    for i := 0; i < len(data); i++ {
        data[i] ^= key
    }
}

func handleSession(f *Forwarder, key string, session *Session, timeout time.Duration) {
    log.Printf("%s started", key)
    data := make([]byte, 1500) // Fixed buffer size

    for {
        n, _, err := session.serverConn.ReadFromUDP(data)
        if err != nil {
            log.Printf("Error while reading from server: %s", err)
            break
        }

        // Encrypt received data before forwarding
        xorEncryptDecrypt(data[:n], 0xAB) // Use any desired key
        
        _, err = f.localConn.WriteToUDP(data[:n], session.clientAddr)
        if err != nil {
            log.Printf("Error while writing to client: %s", err)
            break
        }
        verbosePrintf("Sent %d bytes to %s\n", n, session.clientAddr.String())

        // Update last active time
        session.lastActiveTime = time.Now()
    }

    f.deleteSession(key)
    log.Printf("%s ended", key)
}

func checkInactiveSessions(f *Forwarder, timeout time.Duration) {
    ticker := time.NewTicker(timeout / 2) // Check twice the timeout duration
    defer ticker.Stop()

    for range ticker.C {
        f.mutex.Lock()
        for key, session := range f.sessions {
            // Calculate duration since last activity
            inactiveDuration := time.Since(session.lastActiveTime)
            if inactiveDuration >= timeout {
                log.Printf("Inactive session %s disconnected", key)
                session.serverConn.Close()
                delete(f.sessions, key)
            }
        }
        f.mutex.Unlock()
    }
}

func receivingFromClient(f *Forwarder, timeout time.Duration) {
    data := make([]byte, 1500) // Fixed buffer size
    for {
        n, clientAddr, err := f.localConn.ReadFromUDP(data)
        if err != nil {
            log.Printf("Error during read: %s", err)
            continue
        }
        verbosePrintf("<%s> size: %d\n", clientAddr, n)

        // Create a unique key using client's IP and source port
        key := fmt.Sprintf("%s:%d", clientAddr.IP.String(), clientAddr.Port)

        f.mutex.Lock()
        session, found := f.sessions[key]
        f.mutex.Unlock()

        if found {
            _, err := session.serverConn.Write(data[:n])
            if err != nil {
                log.Printf("Error while writing to server: %s", err)
            }
        } else {
            serverConn, err := net.DialUDP("udp", nil, f.toAddr)
            if err == nil {
                _, err := serverConn.Write(data[:n])
                if err != nil {
                    log.Printf("Error while writing to server (init): %s", err)
                }
                session := Session{
                    clientAddr:    clientAddr,
                    serverConn:    serverConn,
                    lastActiveTime: time.Now(),
                }
                f.addSession(key, &session)
                go handleSession(f, key, &session, timeout)
            } else {
                log.Printf("Error while creating server connection: %s", err)
            }
        }
    }
}

func forward(from string, to string, timeout time.Duration) (*Forwarder, error) {
    fromAddr, err := net.ResolveUDPAddr("udp", from)
    if err != nil {
        return nil, err
    }

    toAddr, err := net.ResolveUDPAddr("udp", to)
    if err != nil {
        return nil, err
    }

    localConn, err := net.ListenUDP("udp", fromAddr)
    if err != nil {
        return nil, err
    }

    f := Forwarder{
        fromAddr:  fromAddr,
        toAddr:    toAddr,
        localConn: localConn,
        sessions:  make(map[string]*Session),
    }

    log.Printf("<%s> forward to <%s>\n", fromAddr.String(), toAddr.String())

    go receivingFromClient(&f, timeout)
    go checkInactiveSessions(&f, timeout)

    return &f, nil
}

// WaitForCtrlC waits for termination signal
func WaitForCtrlC() {
    var endWaiter sync.WaitGroup
    endWaiter.Add(1)
    var signalChannel chan os.Signal
    signalChannel = make(chan os.Signal, 1)
    signal.Notify(signalChannel, os.Interrupt)
    go func() {
        <-signalChannel
        endWaiter.Done()
    }()
    endWaiter.Wait()
}

func main() {
    flag.Parse()
    timeout := *timeoutDuration

    for _, pair := range strings.Split(*fw, ",") {
        fromAndTo := strings.Split(pair, "~")
        if len(fromAndTo) != 2 {
            log.Printf("Invalid from/to format: %s", fromAndTo)
            break
        }
        _, err := forward(fromAndTo[0], fromAndTo[1], timeout)
        if err != nil {
            log.Printf("Error while creating forwarder: %s", err)
            break
        }
    }
    WaitForCtrlC()
}
