package tunnel

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
	mrand "math/rand"
	"crypto/cipher"

	"github.com/xjdrew/gotunnel/shadowstream"
	"crypto/sha256"
	"github.com/jiguorui/crc16"
	"crypto/rand"
	"errors"
)

const (
	MaxReadTimeout        = 120 // seconds
	TunnelPacketSize      = 8192
	TunnelKeepAlivePeriod = time.Second * 180
	PASSWORD              = "Through-the-tunnel-I-reach-the-world"
)

var (
	TByteOrder        = binary.BigEndian
	IdGenMux          sync.Mutex
	TunnelReadTimeout       = uint(60 * 3)
	LogLevel          uint8 = uint8(1)
	mpool                   = NewMPool(TunnelPacketSize)
	ExitOnError             = true // only for test
	VerifyCRC               = true //数据CRC校验.

	CipherName string
)

var errPeerClosed = errors.New("errPeerClosed")
var errClosed = errors.New("closed")
var errWriteClosed = errors.New("writeclosed")
var errReadClosed = errors.New("readclosed")

var errTooLarge = fmt.Errorf("tunnel.Read: packet too large")
var errCRC = fmt.Errorf("error crc in packet Header")
var errPacketId = fmt.Errorf("error PacketId in packet Header")

type TunnelConn interface {
	net.Conn
	Flush() error
	setKeys(token AuthToken, secret string, client bool)
}

type tnConn struct {
	net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
	enc    cipher.Stream
	dec    cipher.Stream
}

var _ TunnelConn = (*tnConn)(nil) // Verify that *T implements I.

func (tn *tnConn) Read(b []byte) (int, error) {
	n, err := tn.reader.Read(b)
	if n > 0 && tn.dec != nil {
		tn.dec.XORKeyStream(b[:n], b[:n])
	}
	return n, err
}

func (tn *tnConn) Write(b []byte) (int, error) {
	if tn.enc != nil {
		tn.enc.XORKeyStream(b, b)
	}
	return tn.writer.Write(b)
}

func (tn *tnConn) Flush() error {
	return tn.writer.Flush()
}

func (tn *tnConn) Close() error {
	tn.Flush()
	return tn.Conn.Close()
}

func (tn *tnConn) setKeys(taa AuthToken, secretStr string, fromClient bool) {

	var encSecret, decSecret [32]byte
	//client
	encSecret = taa.toClientEncKey([]byte(secretStr))
	decSecret = taa.toServerEncKey([]byte(secretStr))

	if !fromClient { // server
		encSecret, decSecret = decSecret, encSecret
	}

	func() {
		encCipher, encKey, err := shadowstream.PickCipher(CipherName, encSecret[:])
		if err != nil {
			panic("bad cipher")
		}
		encIV := sha256.Sum256(Reverse(encKey))
		tn.enc = encCipher.Encrypter(shadowstream.Kdf(encIV[:], encCipher.IVSize()))
	}()

	func() {
		decCipher, decKey, err := shadowstream.PickCipher(CipherName, decSecret[:])
		if err != nil {
			panic("bad cipher")
		}
		decIV := sha256.Sum256(Reverse(decKey))
		tn.dec = decCipher.Decrypter(shadowstream.Kdf(decIV[:], decCipher.IVSize()))

	}()

}

/// tunnel packet Header
/// a tunnel packet consists of a Header and a body
/// Len is the length of subsequent packet body
type Header struct {
	PacketId  uint16
	HeaderCRC uint16
	DataCRC   uint16
	LinkId    uint16
	Len       uint16
}

func hCRC(he *Header) uint16 {
	buff := make([]byte, 8)
	for i, d := range []uint16{he.PacketId, he.DataCRC, he.LinkId, he.Len} {
		TByteOrder.PutUint16(buff[(2 * i):], d)
	}

	return crc16.CheckSum(buff)
}

type Tunnel struct {
	tconn TunnelConn
	// protect concurrent write. 并发write,flush等等均会导致数据错误.
	wlock                sync.Mutex
	werr                 error
	running              bool
	lastFlushMs          int64
	tunId                uint16
	readPacketIdCounter  uint16
	writePacketIdCounter uint16
}

func newTunnel(conn net.Conn) *Tunnel {
	var tun Tunnel
	tun.tconn = &tnConn{conn, bufio.NewReaderSize(conn, TunnelPacketSize*2), bufio.NewWriterSize(conn, TunnelPacketSize*2), nil, nil}
	tun.running = true
	go tun.startAutoFlush()
	return &tun
}

func (tun *Tunnel) Close() error {
	tun.wlock.Lock()
	defer tun.wlock.Unlock()
	if tun.running {
		tun.running = false
		Warn("%s closed", tun)
		return tun.tconn.Close()
	}
	return nil
}

func (tun *Tunnel) Flush() error {
	tun.lastFlushMs = TimeNowMs()
	return tun.tconn.Flush()
}

func (tun *Tunnel) startAutoFlush() {
	interval := time.Millisecond * time.Duration(100+mrand.Intn(40))
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	flushJob := func() bool {
		tun.wlock.Lock()
		defer tun.wlock.Unlock()
		if tun.running && (TimeNowMs()-tun.lastFlushMs > 40) {
			tun.Flush()
		}
		return tun.running
	}

	for range ticker.C {
		running := flushJob()
		if !running {
			break
		}
	}
}


const flushLimitSize = TunnelPacketSize * 7 / 10

/// can write concurrently
func (tun *Tunnel) WritePacket(kid uint16, data []byte, forceFlush bool) (err error) {
	defer mpool.Put(data)

	tun.wlock.Lock()
	defer tun.wlock.Unlock()

	if tun.werr != nil {
		return tun.werr
	}

	//这些字节,废弃一些字节.
	if false {
		dropped := mpool.Get()[0:(4 - tun.writePacketIdCounter%4)]
		rand.Read(dropped)
		if _, err = tun.tconn.Write(dropped); err != nil {
			tun.werr = err
			tun.Close()
			return err
		}
		mpool.Put(dropped)
	}

	// Header
	dataCRC := crc16.CheckSum(data)
	header := Header{tun.writePacketIdCounter, 0, dataCRC, kid, uint16(len(data))}
	header.HeaderCRC = uint16(hCRC(&header))
	if err = binary.Write(tun.tconn, TByteOrder, header); err != nil {
		tun.werr = err
		tun.Close()
		return err
	}
	tun.writePacketIdCounter += 1

	// data
	if _, err = tun.tconn.Write(data); err != nil {
		tun.werr = err
		tun.Close()
		return err
	}

	switch {
	case forceFlush || (!forceFlush && len(data) > flushLimitSize):
		if err = tun.Flush(); err != nil {
			tun.werr = err
			tun.Close()
			return err
		}
	default:
		// not flush
	}

	Debug("write packet %d", header.PacketId)

	return nil
}

/// can't read concurrently
func (tun *Tunnel) ReadPacket() (linkId uint16, data []byte, err error) {
	var h Header

	// 配合心跳ping-pong,检查是否断网
	// A deadline is an absolute time after which I/O operations
	// fail with a timeout (see type Error) instead of
	// blocking. The deadline applies to all future and pending
	// I/O, not just the immediately following call to Read or
	// Write. After a deadline has been exceeded, the connection
	// can be refreshed by setting a deadline in the future.
	// An idle timeout can be implemented by repeatedly extending
	// the deadline after successful Read or Write calls.
	deadLine := time.Now().Add(time.Second * time.Duration(TunnelReadTimeout))
	tun.tconn.SetReadDeadline(deadLine)

	//废弃一些字节.
	if false {
		dropped := mpool.Get()[0:(4 - tun.readPacketIdCounter%4)]
		if _, err = io.ReadFull(tun.tconn, dropped); err != nil {
			return
		}
		mpool.Put(dropped)
	}

	if err = binary.Read(tun.tconn, TByteOrder, &h); err != nil {
		Error("ReadPacket: read Header error: %v", err)
		return
	}
	Debug("ReadPacket read Header: %v", &h)

	if h.PacketId != tun.readPacketIdCounter {
		Error("error PacketId")
		err = errPacketId
		return
	}
	tun.readPacketIdCounter += 1

	if VerifyCRC {
		hcrc := hCRC(&h)
		if h.HeaderCRC != hcrc {
			Error("error HeaderCRC")
			err = errCRC
			return
		}
	}

	if h.Len > TunnelPacketSize {
		err = errTooLarge
		return
	}

	data = mpool.Get()[0:h.Len]
	if _, err = io.ReadFull(tun.tconn, data); err != nil {
		return
	}
	Debug("ReadPacket: read data: %d", len(data))

	if VerifyCRC {
		dataCRC := crc16.CheckSum(data)
		if h.DataCRC != dataCRC {
			Error("error DataCRC")
			err = errCRC
			return
		}
	}

	linkId = h.LinkId
	Debug("ReadPacket: OK")
	return
}

func (tun Tunnel) String() string {
	info := fmt.Sprintf("tunnel(%5d, L%s, R%s)", tun.tunId, tun.tconn.LocalAddr(), tun.tconn.RemoteAddr())
	return info
}
