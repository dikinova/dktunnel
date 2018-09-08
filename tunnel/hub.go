package tunnel

import (
	"bytes"
	"encoding/binary"
	"sync"
	"net"
	"time"
	"io"
	"fmt"
)

const (
	CD_LINK_DATA uint8 = iota
	CD_LINK_CREATE
	CD_LINK_CLOSE
	CD_LINK_CLOSE_WriteErr
	CD_LINK_CLOSE_ReadErr
	CD_HEARTBEAT
)

type Ctrl struct {
	// 为了进行自动序列化操作, 字段一定要导出,大写开头.
	Code   uint8  // control command
	LinkId uint16 // id
}

type Hub struct {
	// Hub比tunnel多了管理Link的功能.
	tunnel *Tunnel

	rwmx   sync.RWMutex // protect links
	links  map[uint16]*Link
	Closed bool

	onCtrlFilter func(cmd Ctrl) bool
}

func newHub(tunnel *Tunnel) *Hub {
	CT(T_Hub, OP_Increase)
	return &Hub{
		tunnel: tunnel,
		links:  make(map[uint16]*Link),
	}
}

func (h *Hub) SendCmd(linkId uint16, code uint8) bool {
	buf := bytes.NewBuffer(mpool.Get()[0:0])
	c := Ctrl{
		Code:   code,
		LinkId: linkId,
	}
	binary.Write(buf, TByteOrder, &c)
	Debug("tun(%5d) link(%d) send cmd:%d data:%v", h.tunnel.tunId, linkId, code, c)
	return h.Send(0, buf.Bytes(), false)
}

func (h *Hub) Send(id uint16, data []byte, forceFlush bool) bool {
	if err := h.tunnel.WritePacket(id, data, forceFlush); err != nil {
		Warn("link(%d) write to %s failed:%s", id, h.tunnel, err.Error())
		return false
	}
	return true
}

func (h *Hub) onCtrl(cmd Ctrl) {
	if h.onCtrlFilter != nil && h.onCtrlFilter(cmd) {
		return
	}

	id := cmd.LinkId
	k := h.getLink(id)
	if k == nil {
		// 有可能远程数据发送过来,但是本地的link已经提前异常退出了.
		Info("link(%d) recv cmd:%d, no link", id, cmd.Code)
		return
	}

	switch cmd.Code {
	case CD_LINK_CLOSE:
		k.closeAll()
	case CD_LINK_CLOSE_WriteErr:
		k.closeRead()
	case CD_LINK_CLOSE_ReadErr:
		k.closeWrite()
	default:
		Error("link(%d) receive unknown cmd:%v", id, cmd)
	}
}

func (h *Hub) onData(id uint16, data []byte) {
	link := h.getLink(id)

	if link == nil {
		mpool.Put(data)
		Info("link(%d) no link", id)
		return
	}

	link.writeChannel(data)
}

///client, server共用此函数
func (h *Hub) Start() {
	defer Recover()
	defer h.Close()
	CT(T_Coroutine, OP_Increase)
	defer CT(T_Coroutine, OP_Decrease)
	Warn("%s start", h.tunnel)
	for {
		linkId, data, err := h.tunnel.ReadPacket()
		if err != nil {
			Warn("%s read failed:%v", h.tunnel, err)
			break
		}

		if linkId == 0 {
			var cmd Ctrl
			buf := bytes.NewBuffer(data)
			err := binary.Read(buf, TByteOrder, &cmd)
			mpool.Put(data)
			//cmd.fromBytes(data)
			if err != nil {
				Error("tun(%5d) parse failed:%s, break dispatch", h.tunnel.tunId, err.Error())
				break
			}
			Debug("tun(%5d) link(%d) recv cmd:%d", h.tunnel.tunId, linkId, cmd.Code)
			h.onCtrl(cmd)
		} else {
			Debug("tun(%5d) link(%d) recv %d bytes data", h.tunnel.tunId, linkId, len(data))
			h.onData(linkId, data)
		}
	}

	// tunnel disconnect, so reset all link
	Warn("%s reset all link", h.tunnel)
	h.closeAllLink()
}

func (h *Hub) Close() {
	h.rwmx.Lock()
	defer h.rwmx.Unlock()
	if !h.Closed {
		h.Closed = true
		h.tunnel.Close()
		CT(T_Hub, OP_Decrease)
	}
}

func (h *Hub) Status(w io.Writer) {
	h.rwmx.RLock()
	defer h.rwmx.RUnlock()
	var links []uint16
	for id := range h.links {
		links = append(links, id)
	}
	fmt.Fprintf(w, "\n<status> %s, links(%d)", h.tunnel, len(h.links))
}

/// client,server共用此函数
func (h *Hub) closeAllLink() {
	h.rwmx.Lock()
	defer h.rwmx.Unlock()

	for _, k := range h.links {
		k.closeAll()
	}

	h.links = make(map[uint16]*Link) // clear links

}

/// hub function
func (h *Hub) getLink(id uint16) *Link {
	h.rwmx.RLock()
	defer h.rwmx.RUnlock()
	return h.links[id]
}

/// 共用
func (h *Hub) deleteLink(id uint16) {
	Info("link(%d) delete", id)
	h.rwmx.Lock()
	defer h.rwmx.Unlock()
	delete(h.links, id)
}

///共用
func (h *Hub) createLink(id uint16) *Link {
	Info("link(%d) new link", id)
	h.rwmx.Lock()
	defer h.rwmx.Unlock()
	if _, ok := h.links[id]; ok {
		Error("link(%d) repeated", id)
		return nil
	}
	l := &Link{
		id:       id,
		wchannel: make(ByteChan, 30),
	}
	CT(T_Link, OP_Increase)
	CT(T_Channel, OP_Increase)
	h.links[id] = l
	return l
}

/// client,server端公用该函数
func (h *Hub) runLink(k *Link, conn *net.TCPConn) {
	conn.SetKeepAlive(true)
	conn.SetKeepAlivePeriod(time.Second * 30)
	k.setConn(conn)

	Info("link(%d) start: %v", k.id, conn.RemoteAddr())
	defer k.closeAll()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { //读入link kconn数据写入tunnel
		defer wg.Done()
		CT(T_Coroutine, OP_Increase)
		defer CT(T_Coroutine, OP_Decrease)

	LOOP:
		for {
			data, err := k.readKConn()
			switch {
			case err == errClosed:
				h.SendCmd(k.id, CD_LINK_CLOSE)
				break LOOP

			case err == errReadClosed:
				h.SendCmd(k.id, CD_LINK_CLOSE_ReadErr)
				break LOOP
			case err != nil:
				Fail()
			case err == nil:
				ok := h.Send(k.id, data, false)
				if !ok {
					break LOOP
				}

			}
		}
	}()

	wg.Add(1)
	go func() { //将数据从 link buffer写入 link conn
		defer wg.Done()
		CT(T_Coroutine, OP_Increase)
		defer CT(T_Coroutine, OP_Decrease)

		defer k.closeWrite()

		for {
			data, ok := <-k.wchannel
			if !ok {
				break
			}

			_, err := k.kconn.Write(data)
			mpool.Put(data)
			if err != nil {
				// need drain wchannel
				h.SendCmd(k.id, CD_LINK_CLOSE_WriteErr)
				break
			}
		}
	}()
	wg.Wait()
	Info("link(%d) close", k.id)
}
