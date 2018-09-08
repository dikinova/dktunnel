//
//   date  : 2015-03-06
//   author: xjdrew
//

package ztests

import (
	"testing"
	"github.com/dikinova/dktunnel/tunnel"
	"github.com/jiguorui/crc16"
	"hash/crc32"
	"encoding/binary"
	"bytes"
	"fmt"
)

func TestBase(t *testing.T) {

	ah := tunnel.Header{
		12, 23, 34, 45, 56,
	}

	var buf bytes.Buffer

	binary.Write(&buf, tunnel.TByteOrder, &ah)

	t.Log(fmt.Sprintf("%x", buf.Bytes()))

}

func TestAuth(t *testing.T) {
	//key := "a test key"

	ta := func(key string) {
		t.Log("key", key)
		a1 := tunnel.NewTaa(key)
		a2 := tunnel.NewTaa(key)

		a1.GenRandomToken()
		a1.Token.Challenge = 100
		a1.Token.Timestamp = 200
		cb := a1.GenCipherBlock(nil)
		t.Log("id",a1.Token.ToID())
		t.Log("chanllenge", a1.Token.Challenge, "ts", a1.Token.Timestamp)
		t.Log("block 1:", cb, "crc16", crc16.CheckSum(cb), "crc32", crc32.ChecksumIEEE(cb))
		if !a1.CheckMAC(cb) {
			t.Fatal("check signature failed")
		}

		b2, err := a2.ExchangeCipherBlock(cb)
		t.Log("block 2:", b2)
		if err != nil {
			t.Fatal("exchange block failed")
		}

		if !a1.VerifyCipherBlock(b2) {
			t.Fatal("verify exchanged block failed")
		}
	}

	ta("123456789")
	ta("987654321")
	ta("abcdefg")
	ta(tunnel.PASSWORD)

}