package proxy

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
)

const (
	handshakeLen = 64
	skipLen      = 8
	preKeyLen    = 32
	ivLen        = 16
	protoTagPos  = 56
	dcIdxPos     = 60
)

var (
	protoTagAbridged     = []byte{0xef, 0xef, 0xef, 0xef}
	protoTagIntermediate = []byte{0xee, 0xee, 0xee, 0xee}
	protoTagSecure       = []byte{0xdd, 0xdd, 0xdd, 0xdd}
)

type Protocol string

const (
	ProtocolAbridged     Protocol = "abridged"
	ProtocolIntermediate Protocol = "intermediate"
	ProtocolSecure       Protocol = "secure"
)

type ClientHello struct {
	DC             int
	IsMedia        bool
	Protocol       Protocol
	ClientDecBytes []byte
}

func ParseClientHello(handshake []byte, secretHex string) (ClientHello, error) {
	var hello ClientHello
	if len(handshake) != handshakeLen {
		return hello, fmt.Errorf("invalid handshake length: %d", len(handshake))
	}

	secret, err := hex.DecodeString(secretHex)
	if err != nil {
		return hello, fmt.Errorf("decode secret: %w", err)
	}

	decPrekeyAndIV := handshake[skipLen : skipLen+preKeyLen+ivLen]
	decPrekey := decPrekeyAndIV[:preKeyLen]
	decIV := decPrekeyAndIV[preKeyLen:]
	decKey := sha256.Sum256(append(append([]byte{}, decPrekey...), secret...))

	block, err := aes.NewCipher(decKey[:])
	if err != nil {
		return hello, err
	}
	stream := cipher.NewCTR(block, decIV)
	decrypted := make([]byte, len(handshake))
	stream.XORKeyStream(decrypted, handshake)

	tag := decrypted[protoTagPos : protoTagPos+4]
	switch {
	case equal4(tag, protoTagAbridged):
		hello.Protocol = ProtocolAbridged
	case equal4(tag, protoTagIntermediate):
		hello.Protocol = ProtocolIntermediate
	case equal4(tag, protoTagSecure):
		hello.Protocol = ProtocolSecure
	default:
		return hello, errors.New("unsupported protocol tag or wrong secret")
	}

	dc := int(int16(binary.LittleEndian.Uint16(decrypted[dcIdxPos : dcIdxPos+2])))
	if dc == 0 {
		return hello, errors.New("invalid dc index 0")
	}
	if dc < 0 {
		hello.DC = -dc
		hello.IsMedia = true
	} else {
		hello.DC = dc
	}
	hello.ClientDecBytes = append([]byte(nil), decPrekeyAndIV...)
	return hello, nil
}

func GenerateRelayInit(protocol Protocol, dc int, isMedia bool) ([]byte, error) {
	init := make([]byte, handshakeLen)

	for {
		if _, err := rand.Read(init); err != nil {
			return nil, err
		}
		if init[0] != 0xef && string(init[:4]) != "HEAD" && string(init[:4]) != "POST" && string(init[:4]) != "GET " {
			break
		}
	}

	dcIndex := dc
	if isMedia {
		dcIndex = -dc
	}

	copy(init[protoTagPos:protoTagPos+4], protocolTag(protocol))
	binary.LittleEndian.PutUint16(init[dcIdxPos:dcIdxPos+2], uint16(int16(dcIndex)))

	encKey := init[skipLen : skipLen+preKeyLen]
	encIV := init[skipLen+preKeyLen : skipLen+preKeyLen+ivLen]

	decInput := append([]byte(nil), init[skipLen:skipLen+preKeyLen+ivLen]...)
	reverseBytes(decInput)
	decKey := decInput[:preKeyLen]
	decIV := decInput[preKeyLen:]

	block, err := aes.NewCipher(decKey)
	if err != nil {
		return nil, err
	}
	stream := cipher.NewCTR(block, decIV)
	encrypted := make([]byte, handshakeLen)
	stream.XORKeyStream(encrypted, init)
	copy(init[protoTagPos:], encrypted[protoTagPos:])

	_ = encKey
	_ = encIV
	return init, nil
}

func NewClientDecryptor(clientDecBytes []byte, secretHex string) (cipher.Stream, cipher.Stream, error) {
	secret, err := hex.DecodeString(secretHex)
	if err != nil {
		return nil, nil, err
	}
	if len(clientDecBytes) != preKeyLen+ivLen {
		return nil, nil, errors.New("invalid client decipher state")
	}

	decPrekey := clientDecBytes[:preKeyLen]
	decIV := clientDecBytes[preKeyLen:]
	decKey := sha256.Sum256(append(append([]byte{}, decPrekey...), secret...))

	encPrekeyIV := append([]byte(nil), clientDecBytes...)
	reverseBytes(encPrekeyIV)
	encPrekey := encPrekeyIV[:preKeyLen]
	encIV := encPrekeyIV[preKeyLen:]
	encKey := sha256.Sum256(append(append([]byte{}, encPrekey...), secret...))

	decBlock, err := aes.NewCipher(decKey[:])
	if err != nil {
		return nil, nil, err
	}
	encBlock, err := aes.NewCipher(encKey[:])
	if err != nil {
		return nil, nil, err
	}

	clientDecryptor := cipher.NewCTR(decBlock, decIV)
	clientEncryptor := cipher.NewCTR(encBlock, encIV)

	zero := make([]byte, handshakeLen)
	clientDecryptor.XORKeyStream(zero, zero)
	return clientDecryptor, clientEncryptor, nil
}

func NewRelayCryptors(relayInit []byte) (cipher.Stream, cipher.Stream, error) {
	if len(relayInit) != handshakeLen {
		return nil, nil, errors.New("invalid relay init length")
	}

	encKey := relayInit[skipLen : skipLen+preKeyLen]
	encIV := relayInit[skipLen+preKeyLen : skipLen+preKeyLen+ivLen]

	decInput := append([]byte(nil), relayInit[skipLen:skipLen+preKeyLen+ivLen]...)
	reverseBytes(decInput)
	decKey := decInput[:preKeyLen]
	decIV := decInput[preKeyLen:]

	encBlock, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, nil, err
	}
	decBlock, err := aes.NewCipher(decKey)
	if err != nil {
		return nil, nil, err
	}

	tgEncryptor := cipher.NewCTR(encBlock, encIV)
	tgDecryptor := cipher.NewCTR(decBlock, decIV)

	zero := make([]byte, handshakeLen)
	tgEncryptor.XORKeyStream(zero, zero)
	return tgEncryptor, tgDecryptor, nil
}

func protocolTag(protocol Protocol) []byte {
	switch protocol {
	case ProtocolAbridged:
		return protoTagAbridged
	case ProtocolIntermediate:
		return protoTagIntermediate
	default:
		return protoTagSecure
	}
}

func equal4(a, b []byte) bool {
	if len(a) < 4 || len(b) < 4 {
		return false
	}
	return a[0] == b[0] && a[1] == b[1] && a[2] == b[2] && a[3] == b[3]
}

func reverseBytes(v []byte) {
	for i := 0; i < len(v)/2; i++ {
		j := len(v) - 1 - i
		v[i], v[j] = v[j], v[i]
	}
}
