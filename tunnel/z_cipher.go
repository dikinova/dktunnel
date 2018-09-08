/*
来自 goshadowsocks2 项目
有少量的改动.
 */
package tunnel

import (
	"crypto/aes"
	"crypto/cipher"
	"strconv"

	"github.com/Yawning/chacha20"
	"crypto/rc4"
	"crypto/sha256"
	"sort"
	"strings"
	"errors"
)

/// Cipher generates a pair of stream ciphers for encryption and decryption.
type Cipher interface {
	IVSize() int
	Encrypter(iv []byte) cipher.Stream
	Decrypter(iv []byte) cipher.Stream
}

type KeySizeError int

func (e KeySizeError) Error() string {
	return "key size error: need " + strconv.Itoa(int(e)) + " bytes"
}

/// CTR mode
type ctrStream struct{ cipher.Block }

func (b *ctrStream) IVSize() int                       { return b.BlockSize() }
func (b *ctrStream) Decrypter(iv []byte) cipher.Stream { return b.Encrypter(iv) }
func (b *ctrStream) Encrypter(iv []byte) cipher.Stream { return cipher.NewCTR(b, iv) }

func AESCTR(key []byte) (Cipher, error) {
	blk, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return &ctrStream{blk}, nil
}

/// CFB mode
type cfbStream struct{ cipher.Block }

func (b *cfbStream) IVSize() int                       { return b.BlockSize() }
func (b *cfbStream) Decrypter(iv []byte) cipher.Stream { return cipher.NewCFBDecrypter(b, iv) }
func (b *cfbStream) Encrypter(iv []byte) cipher.Stream { return cipher.NewCFBEncrypter(b, iv) }

func AESCFB(key []byte) (Cipher, error) {
	blk, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return &cfbStream{blk}, nil
}

/// IETF-variant of chacha20
type chacha20ietfkey []byte

func (k chacha20ietfkey) IVSize() int                       { return chacha20.INonceSize }
func (k chacha20ietfkey) Decrypter(iv []byte) cipher.Stream { return k.Encrypter(iv) }
func (k chacha20ietfkey) Encrypter(iv []byte) cipher.Stream {
	ciph, err := chacha20.NewCipher(k, iv)
	if err != nil {
		panic(err) // should never happen
	}
	return ciph
}

func Chacha20IETF(key []byte) (Cipher, error) {
	if len(key) != chacha20.KeySize {
		return nil, KeySizeError(chacha20.KeySize)
	}
	return chacha20ietfkey(key), nil
}

type xchacha20key []byte

func (k xchacha20key) IVSize() int                       { return chacha20.XNonceSize }
func (k xchacha20key) Decrypter(iv []byte) cipher.Stream { return k.Encrypter(iv) }
func (k xchacha20key) Encrypter(iv []byte) cipher.Stream {
	ciph, err := chacha20.NewCipher(k, iv)
	if err != nil {
		panic(err) // should never happen
	}
	return ciph
}

func Chacha20X(key []byte) (Cipher, error) {
	if len(key) != chacha20.KeySize {
		return nil, KeySizeError(chacha20.KeySize)
	}
	return xchacha20key(key), nil
}

///-----------------------------------------------

type dummy []byte

func dummy_reverse(a byte) byte {
	b := (a << 4) | (a >> 4)
	return b
}

func (k dummy) XORKeyStream(dst, src []byte) {
	for i := range src { //将每一个字节的前后互换.
		dst[i] = dummy_reverse(src[i])
	}
}

func (k dummy) IVSize() int                       { return 0 }
func (k dummy) Decrypter(iv []byte) cipher.Stream { return k.Encrypter(iv) }
func (k dummy) Encrypter(iv []byte) cipher.Stream {
	return k
}

func Dummy(key []byte) (Cipher, error) {
	var d dummy
	return &d, nil
}

/// rc4
type rc4key []byte

func (k rc4key) IVSize() int                       { return 0 }
func (k rc4key) Decrypter(iv []byte) cipher.Stream { return k.Encrypter(iv) }
func (k rc4key) Encrypter(iv []byte) cipher.Stream {
	ciph, err := rc4.NewCipher(k)
	if err != nil {
		panic(err) // should never happen
	}
	return ciph
}

func RC4(key []byte) (Cipher, error) {
	if len(key) != 128 {
		return nil, KeySizeError(128)
	}
	return rc4key(key), nil
}

///-------------------------------------------------
/// List of stream ciphers: key size in bytes and constructor
var streamList = map[string]struct {
	KeySize int
	New     func(key []byte) (Cipher, error)
}{
	"AES-128-CTR":  {16, AESCTR},
	"AES-192-CTR":  {24, AESCTR},
	"AES-256-CTR":  {32, AESCTR},
	"AES-128-CFB":  {16, AESCFB},
	"AES-192-CFB":  {24, AESCFB},
	"AES-256-CFB":  {32, AESCFB},
	"CHACHA20IETF": {32, Chacha20IETF},
	"CHACHA20X":    {32, Chacha20X},
	// only for demo
	"DUMMY": {8, Dummy},
	// rc4最多支持256字节密钥. rc4已经是不安全的加密算法.
	"RC4-128": {128, RC4},
	"RC4-256": {256, RC4},
}

/// ListCipher returns a list of available cipher names
func ListCipher() string {
	var l []string
	for k := range streamList {
		l = append(l, k)
	}
	sort.Strings(l)
	return strings.Join(l, " ")
}

/// ErrCipherNotSupported occurs when a cipher is not supported
var ErrCipherNotSupported = errors.New("cipher not supported")

/// PickCipher returns a Cipher of the given name. Derive key.
func PickCipher(name string, /*key []byte,*/ password []byte) (Cipher, []byte, error) {
	name = strings.ToUpper(name)

	if choice, ok := streamList[name]; ok {
		key := Kdf(password, choice.KeySize)
		if len(key) != choice.KeySize {
			return nil, key, KeySizeError(choice.KeySize)
		}
		ciph, err := choice.New(key)
		return ciph, key, err
	}

	return nil, nil, ErrCipherNotSupported
}

/// key-derivation function
func Kdf(password []byte, keyLen int) []byte {
	var b, prev []byte
	h := sha256.New()
	for len(b) < keyLen {
		h.Write(prev)
		h.Write(password)
		b = h.Sum(b)
		prev = b[len(b)-h.Size():]
		h.Reset()
	}
	return b[:keyLen]
}
