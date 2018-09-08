package tunnel

import (
	"net"
	"sync"
	"io"
	"time"
)

type ByteChan chan []byte

type Link struct {
	id          uint16
	kconn       *net.TCPConn
	wchannel    ByteChan // write buffer
	writeClosed bool
	readClosed  bool
	closeOnce   sync.Once
	lock        sync.Mutex // protects below fields
}

func (k *Link) writeChannel(data []byte) error {
	k.lock.Lock()
	defer k.lock.Unlock()
	if k.writeClosed {
		mpool.Put(data)
		return errWriteClosed
	}
	k.wchannel <- data
	return nil
}

/// stop read data from link
func (k *Link) closeRead() {
	k.lock.Lock()
	defer k.lock.Unlock()
	if k.readClosed {
		return
	}
	k.readClosed = true
	if k.kconn != nil {
		k.kconn.CloseRead()
	}
	k.tryToCloseKConn()
}

/// stop write data into link
func (k *Link) closeWrite() {
	k.lock.Lock()
	defer k.lock.Unlock()
	if k.writeClosed {
		return
	}
	k.writeClosed = true

	if k.kconn != nil {
		k.kconn.CloseWrite()
	}

	close(k.wchannel)
	drain := func() {
		for data := range k.wchannel {
			mpool.Put(data)
		}
	}
	drain()
	CT(T_Channel, OP_Decrease)
	k.tryToCloseKConn()

}

func (k *Link) tryToCloseKConn() {
	if (k.readClosed && k.writeClosed && k.kconn != nil) {
		k.kconn.Close()
	}
}

/// close link
func (k *Link) closeAll() {
	k.closeOnce.Do(func() {
		k.closeRead()
		k.closeWrite()
		CT(T_Link, OP_Decrease)
	})

}

/// read data from link
func (k *Link) readKConn() ([]byte, error) {

	switch {
	case k.readClosed && k.writeClosed:
		return nil, errClosed
	case k.readClosed:
		return nil, errReadClosed
	}

	b := mpool.Get()
	deadLine := time.Now().Add(time.Second * time.Duration(60*5))
	k.kconn.SetReadDeadline(deadLine)
	n, err := k.kconn.Read(b)

	switch {
	case err == io.EOF:
		k.closeRead()
		mpool.Put(b)
		return nil, errReadClosed
	case err != nil:
		k.closeAll()
		mpool.Put(b)
		return nil, errClosed
	}

	return b[:n], nil
}

/// set low level connection
func (k *Link) setConn(conn *net.TCPConn) {
	if k.kconn != nil {
		Panic("link(%d) repeated set conn", k.id)
	}
	k.kconn = conn
}

var (
	linkIdCounter = uint16(0)
)

func LinkNextId() uint16 {
	IdGenMux.Lock()
	defer IdGenMux.Unlock()
	linkIdCounter += 1
	if linkIdCounter == 0 {
		linkIdCounter = 1
	}
	return linkIdCounter
}
