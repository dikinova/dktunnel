package tunnel

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"hash"

	"crypto/rand"
	"errors"
	"github.com/jiguorui/crc16"
	"encoding/hex"
	"fmt"
)

const (
	TaaTokenSize int = aes.BlockSize             // 16 bytes
	TaaMACSize   int = md5.Size                  // 16 bytes
	TaaBlockSize int = TaaTokenSize + TaaMACSize // 32 bytes
)

type AuthToken struct {
	Challenge uint64 // random number
	Timestamp uint64 // time nonce
}

func (t *AuthToken) ToID() uint16 {
	stb := md5.Sum(t.toBytes())
	var s uint16
	for _, n := range stb {
		s += uint16(n)
	}

	return s
}

func (t *AuthToken) toBytes() []byte { // 16 bytes
	buf := make([]byte, TaaTokenSize)
	TByteOrder.PutUint64(buf, t.Challenge)
	TByteOrder.PutUint64(buf[8:], t.Timestamp)
	return buf
}

/// 从安全的角度,将client和server的 stream cipher分为2个.
func (t *AuthToken) toClientEncKey(secret []byte) [32]byte {
	a := t.toBytes()           //16 bytes
	s := sha256.Sum256(secret) //32 bytes
	a = append(a, s[:]...)
	return sha256.Sum256(a)
}

func (t *AuthToken) toServerEncKey(secret []byte) [32]byte {
	a := t.toBytes()           //16 bytes
	s := sha256.Sum256(secret) //32 bytes
	a = append(s[:], a...)
	return sha256.Sum256(a)
}

func (t *AuthToken) loadBytes(buf []byte) {
	t.Challenge = TByteOrder.Uint64(buf)
	t.Timestamp = TByteOrder.Uint64(buf[8:])
}

/// complement
func (t *AuthToken) complement() AuthToken {
	return AuthToken{
		Challenge: ^t.Challenge,
		Timestamp: ^t.Timestamp,
	}
}

/// is complementary
func (t *AuthToken) isComplementary(t1 AuthToken) bool {
	if t.Challenge != ^t1.Challenge || t.Timestamp != ^t1.Timestamp {
		return false
	}
	return true
}

/// gotunnel auth algorithm
type authTaa struct {
	cipher cipher.Block
	mac    hash.Hash
	Token  AuthToken
}

func NewTaa(secret string) *authTaa {
	s := sha256.Sum256([]byte(secret))
	cipher, _ := aes.NewCipher(s[:TaaTokenSize])
	mac := hmac.New(md5.New, s[TaaTokenSize:])
	return &authTaa{
		cipher: cipher,
		mac:    mac,
	}
}

func init() { // package init function
}

/// generate new token
func (a *authTaa) GenRandomToken() {
	a.Token.Challenge = uint64(SecureRandInt64())
	//增加token的随机性.用时间戳的话,随机性太差.
	a.Token.Timestamp = uint64(SecureRandInt64())
}

/// generate cipher block. EtM mode.
func (a *authTaa) GenCipherBlock(token *AuthToken) []byte {
	if token == nil {
		token = &a.Token
	}

	dst := make([]byte, TaaBlockSize)
	a.cipher.Encrypt(dst, token.toBytes())
	a.mac.Write(dst[:TaaTokenSize])
	sign := a.mac.Sum(nil)
	a.mac.Reset()

	copy(dst[TaaTokenSize:], sign)
	Debug("cipherblock, c:%v t:%v dst:%v", token.Challenge, token.Timestamp, dst)
	return dst
}

func (a *authTaa) CheckMAC(src []byte) bool {
	a.mac.Write(src[:TaaTokenSize])
	expectedMac := a.mac.Sum(nil)
	a.mac.Reset()
	return hmac.Equal(src[TaaTokenSize:], expectedMac)
}

/// exchange cipher block
func (a *authTaa) ExchangeCipherBlock(src []byte) ([]byte, error) {
	Debug("cipher block crc:%d", crc16.CheckSum(src))
	if len(src) != TaaBlockSize {
		return nil, errors.New("BAD LEN")
	}

	if !a.CheckMAC(src) {
		return nil, errors.New("BAD MAC")
	}

	dst := make([]byte, TaaTokenSize)
	a.cipher.Decrypt(dst, src)
	(&a.Token).loadBytes(dst) //解码之后设置token

	// complement challenge 做一个简单的反转,生成确认消息
	token := a.Token.complement()
	return a.GenCipherBlock(&token), nil
}

/// verify cipher block
func (a *authTaa) VerifyCipherBlock(src []byte) bool {
	if len(src) != TaaBlockSize {
		return false
	}

	if !a.CheckMAC(src) {
		return false
	}

	var token AuthToken
	dst := make([]byte, TaaTokenSize)
	a.cipher.Decrypt(dst, src)
	(&token).loadBytes(dst)
	return a.Token.isComplementary(token)
}

func BytesToUInt64(buf []byte) uint64 {
	return TByteOrder.Uint64(buf[0:8])
}

func SecureRandInt64() uint64 {
	b := make([]byte, 8)
	rand.Read(b)
	return BytesToUInt64(b)
}

type HelloA struct {
	Now         uint64
	SecretCRC16 uint16
	Salt        [32]byte
	Hash        [32]byte
}

func newHelloA(secret string) HelloA {
	now := uint64(TimeNowMs())
	rs := make([]byte, 32);
	rand.Read(rs)
	salt := sha256.Sum256(rs)
	str := secret + "," + fmt.Sprintf("%d", now) + "," + hex.EncodeToString(salt[:])
	Info("helloa str:%s", str)
	hash := sha256.Sum256([]byte(str))

	a := HelloA{
		Now:         now,
		SecretCRC16: crc16.CheckSum([]byte(secret)),
		Salt:        salt,
		Hash:        hash,
	}
	return a
}

func (a *HelloA) toBytes() []byte {
	len := 8 + 2 + 32 + 32
	buf := make([]byte, len)
	TByteOrder.PutUint64(buf[:8], a.Now)
	TByteOrder.PutUint16(buf[8:(8 + 2)], a.SecretCRC16)
	copy(buf[10:(10 + 32)], a.Salt[:])
	copy(buf[(10 + 32):len], a.Hash[:])
	return buf
}
