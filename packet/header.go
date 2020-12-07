package packet

import (
	"encoding/binary"
)

func headerLen(rl int) int {
	// add packet type and flag byte + remaining length
	return 1 + varUintLen(uint64(rl))
}

func headerEncode(dst []byte, flags byte, rl int, tl int, t Type) (int, error) {
	// check buffer length
	if len(dst) < tl {
		return 0, makeError(t, "insufficient buffer size, expected %d, got %d", tl, len(dst))
	}

	// check remaining length
	if rl > maxVarUint || rl < 0 {
		return 0, makeError(t, "remaining length (%d) out of bound (max %d, min 0)", rl, maxVarUint)
	}

	// check header length
	hl := headerLen(rl)
	if len(dst) < hl {
		return 0, makeError(t, "insufficient buffer size, expected %d, got %d", hl, len(dst))
	}

	// write type and flags
	typeAndFlags := byte(t)<<4 | (t.defaultFlags() & 0xf)
	typeAndFlags |= flags
	dst[0] = typeAndFlags

	// write remaining length
	n := binary.PutUvarint(dst[1:], uint64(rl))

	return 1 + n, nil
}

func headerDecode(src []byte, t Type) (int, byte, int, error) {
	// check buffer size
	if len(src) < 2 {
		return 0, 0, 0, makeError(t, "insufficient buffer size, expected %d, got %d", 2, len(src))
	}

	// read type and flags
	decodedType := Type(src[0] >> 4)
	flags := src[0] & 0x0f
	total := 1

	// check against static type
	if decodedType != t {
		return total, 0, 0, makeError(t, "invalid type %d", decodedType)
	}

	// check flags except for publish packets
	if t != PUBLISH && flags != t.defaultFlags() {
		return total, 0, 0, makeError(t, "invalid flags, expected %d, got %d", t.defaultFlags(), flags)
	}

	// read remaining length
	_rl, m := binary.Uvarint(src[total:])
	rl := int(_rl)
	total += m

	// check resulting remaining length
	if m <= 0 {
		return total, 0, 0, makeError(t, "error reading remaining length")
	}

	// check remaining length
	if rl > maxVarUint {
		return total, 0, 0, makeError(t, "remaining length (%d) out of bound (max %d, min 0)", rl, maxVarUint)
	}

	// check remaining buffer
	if rl > len(src[total:]) {
		return total, 0, 0, makeError(t, "remaining length (%d) is greater than remaining buffer (%d)", rl, len(src[total:]))
	}

	return total, flags, rl, nil
}
