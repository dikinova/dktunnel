package ztests

import (
	"testing"
	"github.com/jiguorui/crc16"
	"fmt"
)

func TestCRC16(t *testing.T) {

	t.Log(fmt.Sprintf("%x",crc16.CheckSum([]byte("123456789"))))
	t.Log(fmt.Sprintf("%x",crc16.CheckSum([]byte("987654321"))))
	t.Log(fmt.Sprintf("%x",crc16.CheckSum([]byte("abcdefg"))))
	//fmt.Println(tunnel.CRC16UsMB2(m_data))

}