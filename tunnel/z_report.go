package tunnel

import (
	"time"
	"bytes"
	"fmt"
	"sync"
)

func Report(app APP) {
	tc := time.Tick(time.Second * 120)
	for range tc {
		var b bytes.Buffer
		app.Status(&b)
		Warn("status: %s", b.String())
		Warn("%s", CTtoString())
	}
}

type TType uint8
type OP uint8
type CTItem struct {
	tt TType
	op OP
}

func (c CTItem) String() string {
	return fmt.Sprintf(ttNames[c.tt] + "_" + opNames[c.op])
}

const (
	T_Hub TType = iota
	T_Link
	T_Channel
	T_Coroutine
	T_Buf
)

const (
	OP_Increase OP = iota
	OP_Decrease
)

var (
	ttNames          = []string{"Hub", "Link", "Chan", "Crt", "Buf"}
	opNames          = []string{"Inc", "Dec"}
	mapKeys []string = func() []string {
		var ks []string
		for _, tt := range ttNames {
			for _, op := range opNames {
				ks = append(ks, tt+"_"+op)
			}
		}
		return ks
	}()
	ctMux sync.Mutex
	CTmap = make(map[string]uint64)
)

func CT(tt TType, op OP) {
	ctMux.Lock()
	defer ctMux.Unlock()
	CTmap[CTItem{tt, op}.String()]++
}

func CTtoString() string {
	ctMux.Lock()
	defer ctMux.Unlock()
	return mapToStr(CTmap)
}

func mapToStr(m map[string]uint64) string {
	var s string
	for _, k := range mapKeys {
		s += (" " + k + ":" + fmt.Sprint(m[k]))
	}
	return s
}
