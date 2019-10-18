package genetlink

import (
	"encoding/binary"
	"os"
	"sync"
	"unsafe"
)

var (
	sysEndian binary.ByteOrder
	sysOnce   sync.Once

	sysPageSize = os.Getpagesize()
)

// getSysEndian returns byte order in current host.
func getSysEndian() binary.ByteOrder {
	sysOnce.Do(func() {
		buf := [2]byte{}
		*(*uint16)(unsafe.Pointer(&buf[0])) = uint16(0x1234)

		switch buf[0] {
		case 0x34:
			sysEndian = binary.LittleEndian
		default:
			sysEndian = binary.BigEndian
		}
	})
	return sysEndian
}
