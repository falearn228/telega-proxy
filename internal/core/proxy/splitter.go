package proxy

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
)

type MessageSplitter struct {
	dec       cipher.Stream
	protocol  Protocol
	cipherBuf []byte
	plainBuf  []byte
	disabled  bool
}

func NewMessageSplitter(relayInit []byte, protocol Protocol) *MessageSplitter {
	block, err := aes.NewCipher(relayInit[skipLen : skipLen+preKeyLen])
	if err != nil {
		return nil
	}
	dec := cipher.NewCTR(block, relayInit[skipLen+preKeyLen:skipLen+preKeyLen+ivLen])
	zero := make([]byte, handshakeLen)
	dec.XORKeyStream(zero, zero)
	return &MessageSplitter{dec: dec, protocol: protocol}
}

func (s *MessageSplitter) Split(chunk []byte) [][]byte {
	if s == nil || len(chunk) == 0 {
		return nil
	}
	if s.disabled {
		return [][]byte{append([]byte(nil), chunk...)}
	}

	s.cipherBuf = append(s.cipherBuf, chunk...)
	plainChunk := make([]byte, len(chunk))
	s.dec.XORKeyStream(plainChunk, chunk)
	s.plainBuf = append(s.plainBuf, plainChunk...)

	var parts [][]byte
	for len(s.cipherBuf) > 0 {
		packetLen := s.nextPacketLen()
		if packetLen == 0 {
			parts = append(parts, append([]byte(nil), s.cipherBuf...))
			s.cipherBuf = s.cipherBuf[:0]
			s.plainBuf = s.plainBuf[:0]
			s.disabled = true
			break
		}
		if packetLen < 0 {
			break
		}
		parts = append(parts, append([]byte(nil), s.cipherBuf[:packetLen]...))
		s.cipherBuf = s.cipherBuf[packetLen:]
		s.plainBuf = s.plainBuf[packetLen:]
	}
	return parts
}

func (s *MessageSplitter) Flush() [][]byte {
	if s == nil || len(s.cipherBuf) == 0 {
		return nil
	}
	tail := append([]byte(nil), s.cipherBuf...)
	s.cipherBuf = s.cipherBuf[:0]
	s.plainBuf = s.plainBuf[:0]
	return [][]byte{tail}
}

func (s *MessageSplitter) nextPacketLen() int {
	if len(s.plainBuf) == 0 {
		return -1
	}
	switch s.protocol {
	case ProtocolAbridged:
		return s.nextAbridgedLen()
	case ProtocolIntermediate, ProtocolSecure:
		return s.nextIntermediateLen()
	default:
		return 0
	}
}

func (s *MessageSplitter) nextAbridgedLen() int {
	first := s.plainBuf[0]
	var payloadLen int
	var headerLen int
	if first == 0x7F || first == 0xFF {
		if len(s.plainBuf) < 4 {
			return -1
		}
		payloadLen = int(uint32(s.plainBuf[1])|uint32(s.plainBuf[2])<<8|uint32(s.plainBuf[3])<<16) * 4
		headerLen = 4
	} else {
		payloadLen = int(first&0x7F) * 4
		headerLen = 1
	}
	if payloadLen <= 0 {
		return 0
	}
	packetLen := headerLen + payloadLen
	if len(s.plainBuf) < packetLen {
		return -1
	}
	return packetLen
}

func (s *MessageSplitter) nextIntermediateLen() int {
	if len(s.plainBuf) < 4 {
		return -1
	}
	payloadLen := int(binary.LittleEndian.Uint32(s.plainBuf[:4]) & 0x7FFFFFFF)
	if payloadLen <= 0 {
		return 0
	}
	packetLen := 4 + payloadLen
	if len(s.plainBuf) < packetLen {
		return -1
	}
	return packetLen
}
