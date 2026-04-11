package proxy

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"testing"
)

func TestGenerateRelayInitEncryptsTailForTelegramDirection(t *testing.T) {
	cases := []struct {
		name     string
		protocol Protocol
		dc       int
		isMedia  bool
		wantDC   int16
	}{
		{name: "abridged", protocol: ProtocolAbridged, dc: 2, wantDC: 2},
		{name: "intermediate media", protocol: ProtocolIntermediate, dc: 4, isMedia: true, wantDC: -4},
		{name: "secure", protocol: ProtocolSecure, dc: 203, wantDC: 203},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			init, err := GenerateRelayInit(tc.protocol, tc.dc, tc.isMedia)
			if err != nil {
				t.Fatalf("GenerateRelayInit() error = %v", err)
			}

			block, err := aes.NewCipher(init[skipLen : skipLen+preKeyLen])
			if err != nil {
				t.Fatalf("aes.NewCipher() error = %v", err)
			}
			stream := cipher.NewCTR(block, init[skipLen+preKeyLen:skipLen+preKeyLen+ivLen])
			decrypted := make([]byte, len(init))
			stream.XORKeyStream(decrypted, init)

			if got, want := decrypted[protoTagPos:protoTagPos+4], protocolTag(tc.protocol); !equal4(got, want) {
				t.Fatalf("protocol tag = %x, want %x", got, want)
			}
			if got := int16(binary.LittleEndian.Uint16(decrypted[dcIdxPos : dcIdxPos+2])); got != tc.wantDC {
				t.Fatalf("dc index = %d, want %d", got, tc.wantDC)
			}
		})
	}
}
