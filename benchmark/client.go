package main
import (
    "fmt"
    "time"
    "errors"
    "flag"
    "sync"
    "crypto/tls"
    "math/rand"
    "os"

    quic "github.com/lucas-clemente/quic-go"
	"github.com/lucas-clemente/quic-go/utils"
	"github.com/lucas-clemente/quic-go/protocol"
)

type Client struct {
    mutex             sync.RWMutex
    hostname string
    config *quic.Config
	cryptoChangedCond sync.Cond
	session       quic.Session
	encryptionLevel protocol.EncryptionLevel
	terminated bool
    sizeMutex sync.Mutex
	totalSize int
}

func (c *Client) Dial() error {
    _, err := quic.DialAddr(c.hostname, c.config)
    if err != nil {
        utils.Errorf("Dial Failed")
        return err
    }

    /*
    stream, err := session.OpenStream()
    if err != nil {
        utils.Errorf("OpenStream Failed")
        return err
    }

    stream.Write([]byte{'f', 'u', 'c'})

    fmt.Println("StreamId: ", stream.StreamID())
    */

    return err
}

func (c *Client) connStateCallback(sess quic.Session, state quic.ConnState) {
    c.mutex.Lock()
    if c.session == nil {
        c.session = sess
    }
    switch state {
    case quic.ConnStateVersionNegotiated:
        //utils.Debugf("ConnStateVersionNegotiated")
        err := c.versionNegotiateCallback()
        if err != nil {
            c.Close(err)
        }
    case quic.ConnStateSecure:
        c.encryptionLevel = protocol.EncryptionSecure
        utils.Debugf("ConnStateSecure")
        c.cryptoChangedCond.Broadcast()
    case quic.ConnStateForwardSecure:
        c.encryptionLevel = protocol.EncryptionForwardSecure
        utils.Debugf("ConnStateForwardSecure")
        c.cryptoChangedCond.Broadcast()
    }
    c.mutex.Unlock()
}

func (c *Client) versionNegotiateCallback() error {
    //fmt.Println("versionNegotiateCallback")
    return nil
	var err error
	// once the version has been negotiated, open the header stream
	headerStream, err := c.session.OpenStream()
	if err != nil {
		return err
	}
	if headerStream.StreamID() != 3 {
		return errors.New("h2quic Client BUG: StreamID of Header Stream is not 3")
	}

    //headerStream.Write([]byte{'f', 'u', 'c'})
	//c.requestWriter = newRequestWriter(c.headerStream)
	//go c.handleHeaderStream()
    go c.handleStream(headerStream)
	return nil
}

func (c *Client) handleStream(stream quic.Stream) {
    fmt.Println("StreamId ", stream.StreamID())
    c.mutex.Lock()
    //for c.encryptionLevel != protocol.EncryptionForwardSecure {
    //    c.cryptoChangedCond.Wait()
    //}
    c.mutex.Unlock()
    for i:=0; i<10; i++ {
        stream.Write([]byte{'a', 'u', 'c'})
        time.Sleep(1*time.Second)
        fmt.Println(time.Second)
    }
    stream.Close()
}

func (c *Client) DoRequest(PacketSize int) int {
    //fmt.Println(c.encryptionLevel)
    //return nil
    c.mutex.Lock()

    for c.encryptionLevel != protocol.EncryptionForwardSecure {
        c.cryptoChangedCond.Wait()
    }
    //fmt.Println("encryptionLevel == protocol.EncryptionForwardSecure")

    dataStream, err := c.session.OpenStream()
    if err != nil {
        c.Close(err)
        c.mutex.Unlock()
        return 0
    }
    c.mutex.Unlock()

    fmt.Println("Open Data StreamId ", dataStream.StreamID())

	size := 0
    buf := make([]byte, PacketSize)
	for i:=0; !c.terminated; i++ {
		buf[0] = byte(rand.Intn(100))
		cnt, err := dataStream.Write(buf)
		if cnt == 0 || err == nil {
			//size += cnt
			size = cnt
			c.sizeMutex.Lock()
			c.totalSize += size
			c.sizeMutex.Unlock()
		} else {
			fmt.Printf("cnt %v err %v\n", cnt, err)
			c.terminated = true
		}
	}

	fmt.Printf("Stream %d Size %d\n", dataStream.StreamID(), size)

	dataStream.Close()

    return size
}

func main() {
    fmt.Println("QUIC Client")
    defer fmt.Println("QUIC Client Exit")

	n := flag.Int("n", 1, "stream count")
	lossRate := flag.Int("r", 0, "loss rate")
	verbose := flag.Bool("v", false, "verbose")
	flag.Parse()
	if *verbose {
		utils.SetLogLevel(utils.LogLevelDebug)
	} else {
		utils.SetLogLevel(utils.LogLevelInfo)
	}

    client := Client {
        hostname: "www.acevest.com:6121",
        encryptionLevel: protocol.EncryptionUnencrypted,
//        encryptionLevel: protocol.EncryptionSecure,
    }

    var err error
    certs := make([]tls.Certificate, 1)
    prefix := os.Getenv("GOPATH")
    prefix += "/src/github.com/lucas-clemente/quic-go/benchmark/"
    certFile := prefix + "server.pem"
    keyFile  := prefix + "server.key"
    certs[0], err = tls.LoadX509KeyPair(certFile, keyFile)
    if err != nil {
        utils.Errorf("LoadX509KeyPairFail")
        return 
    }
    tlsCfg := &tls.Config {
        Certificates: certs,
    }

    client.config = &quic.Config{
        ConnState: client.connStateCallback,
        TLSConfig: tlsCfg,
    }
    client.cryptoChangedCond = sync.Cond{L: &client.mutex}
	client.terminated = false
    client.Dial()

    var wg sync.WaitGroup
	const PacketSize = 4096
	client.totalSize = 0

	bgnTs := time.Now()
    for i:=0; i<*n; i++ {
		wg.Add(1)
        go func(i int) {
			defer wg.Done()
			client.DoRequest(PacketSize)
		}(i)
    }

	go func() {
		for {
			deltaTs := float32(time.Since(bgnTs))/float32(time.Second)
			if deltaTs > 100.0 {
				client.terminated = true
				deltaTs := float32(time.Since(bgnTs))/float32(time.Second)
				fmt.Printf("QUIC Bandwidth %.2f Mbit/s Loss Rate %d%% Total Size %d Delta Time %.2f s\n", float32(client.totalSize)*8/(1024*1024*deltaTs), *lossRate, client.totalSize, deltaTs)
				client.Close(nil)
				os.Exit(0)
				break
			}
			time.Sleep(100*time.Millisecond)
		}
	}()


	//deltaTs := float32(time.Since(bgnTs))/float32(time.Second)
	//fmt.Printf("QUIC Bandwidth %.2f Mbit/s\n", float32(totalSize)*8/(1024*1024*deltaTs))

	wg.Wait()

	deltaTs := float32(time.Since(bgnTs))/float32(time.Second)
	fmt.Printf("QUIC Bandwidth %.2f Mbit/s Loss Rate %d%% Total Size %d Delta Time %.2f s\n", float32(client.totalSize)*8/(1024*1024*deltaTs), *lossRate, client.totalSize, deltaTs)
	client.Close(nil)
}

func (c *Client) Close(e error) {
	_ = c.session.Close(e)
}
