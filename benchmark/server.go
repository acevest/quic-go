package main

import (
    "fmt"
    "sync"
    "os"
    "io"
    "flag"
    "time"
    "crypto/tls"
    quic "github.com/lucas-clemente/quic-go"
	"github.com/lucas-clemente/quic-go/utils"
	"github.com/lucas-clemente/quic-go/qerr"
)


type Server struct {
    addr string
    certFile string
    keyFile string
    listenerMutex sync.Mutex
    listener    quic.Listener
    sizeChannel chan int
}

func (s *Server) ListenAndServe() error {
    var err error
    certs := make([]tls.Certificate, 1)
    prefix := os.Getenv("GOPATH")
    prefix += "/src/github.com/lucas-clemente/quic-go/benchmark/"
    certFile := prefix + "server.pem"
    keyFile  := prefix + "server.key"
    certs[0], err = tls.LoadX509KeyPair(certFile, keyFile)
    if err != nil {
        return err
    }


    tlsCfg := &tls.Config {
        Certificates: certs,
    }

    config := quic.Config {
        TLSConfig: tlsCfg,
        ConnState: func(session quic.Session, connState quic.ConnState) {
            switch connState {
            case quic.ConnStateVersionNegotiated:
                fmt.Println("QUIC conn state: ConnStateVersionNegotiated");
                go s.handleSession(session)
            case quic.ConnStateInitial:
                fmt.Println("QUIC conn state: ConnStateInitial");
            case quic.ConnStateSecure:
                fmt.Println("QUIC conn state: ConnStateSecure");
            case quic.ConnStateForwardSecure:
                fmt.Println("QUIC conn state: ConnStateForwardSecure");
            }
        },
    }

    var quicListener quic.Listener
    quicListener, err = quic.ListenAddr(s.addr, &config)
    if err != nil {
        return err
    }

    s.listener = quicListener

    return s.listener.Serve()
}

func (s *Server) handleSession(session quic.Session) {
    for {
        stream, err := session.AcceptStream()
        if err != nil {
            utils.Errorf("handle Stream Accept Stream failed ");
            session.Close(qerr.Error(qerr.InvalidHeadersStreamData, err.Error()))
            return
        }
        fmt.Println("Accept New Stream ", stream.StreamID())

        go func() {
            if err := s.handleStream(session, stream); err != nil {
                if _, ok := err.(*qerr.QuicError); !ok {
                    utils.Errorf("error handling client stream request: %s", err.Error())
                }
                stream.Close()
                return
            }

/*
            dataStream, err := session.GetOrOpenStream(protocol.StreamID(stream.StreamID()))
            if err != nil {
                utils.Errorf("Can not get data stream")
                continue
            }

            if dataStream == nil {
                utils.Errorf("data stream is nil")
                continue
            }

            dataStream.Write([]byte('s', 'v', 'r'))
            */
        }()
    }
}

func (s *Server) handleStream(session quic.Session, stream quic.Stream) error {
    cachedSize := 0
    for {
        const maxFrameLen = 4096
        var buf = make([]byte, maxFrameLen)
        //fmt.Println("handle stream id ", stream.StreamID())
        _, err := io.ReadFull(stream, buf[:maxFrameLen])
        if err != nil {
            fmt.Printf("Read Error %s\n", err.Error())
            return err
        }
        /*
        fmt.Println(buf)
        for _, c := range(buf) {
            fmt.Printf("Data: %c\n", c)
        }*/

        cachedSize += maxFrameLen
        select {
        case s.sizeChannel<-cachedSize:
            cachedSize = 0
        default:
        }
    }

    return nil
}

func (s *Server) stat() {
    totalSize := 0
    bgnTs := time.Now()

    for {
        select {
        case n := <-s.sizeChannel:
            if totalSize == 0  {
                bgnTs = time.Now()
            }

            totalSize += n

        default:
        }

        if totalSize != 0 {
            //fmt.Println(float32(time.Since(bgnTs))/float32(time.Second))
            deltaTs := float32(time.Since(bgnTs))/float32(time.Second)

            fmt.Printf("CurrentSpeed %.2f Mbit/s\n", float32(totalSize)*8/(1024*1024*deltaTs))
        }
        time.Sleep(1*time.Second)
    }
}

func main() {
    fmt.Println("QUIC Server...")
    defer fmt.Println("QUIC Server Exit...")
    host := flag.String("h", "127.0.0.1", "host")
	verbose := flag.Bool("v", false, "verbose")
    flag.Parse()

	if *verbose {
		utils.SetLogLevel(utils.LogLevelDebug)
	} else {
		utils.SetLogLevel(utils.LogLevelInfo)
	}

    addr := *host + ":6121"
    fmt.Println("Listen at:", addr)

    var listenerMutex sync.Mutex
    var listener quic.Listener

    quicServer := Server {
        //addr: "localhost:6121",
        addr: addr,
        certFile: "../example/fullchain.pem",
        keyFile: "../example/privkey.pem",
        listenerMutex: listenerMutex,
        listener: listener,
        sizeChannel: make(chan int, 1),
    }


    var wg sync.WaitGroup

    wg.Add(1)

    go func() {
        defer wg.Done()
        quicServer.ListenAndServe()
    }()



    go quicServer.stat()

    wg.Wait()
}
