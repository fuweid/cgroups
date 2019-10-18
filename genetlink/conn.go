package genetlink

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"syscall"
)

var (
	// ErrInvalidErrorCode indicates the error code is invalid, like
	// missing bytes in message.
	ErrInvalidErrorCode = errors.New("genetlink: invalid error code in message")

	// ErrMismatchedSequence indicates the sequence in reply message must
	// be equal to request's sequence.
	ErrMismatchedSequence = errors.New("genetlink: mismatched sequence in message")

	// ErrMismatchedPortID indicates the port ID in reply message must
	// be equal to request's port ID.
	ErrMismatchedPortID = errors.New("genetlink: mismatched port ID in message")

	// ErrUnexpectedDoneMsg indicates that only receive DONE with empty
	// message.
	ErrUnexpectedDoneMsg = errors.New("genetlink: only receive DONE message")

	// ErrUsingClosedConn indicates using closed connection.
	ErrUsingClosedConn = errors.New("genetlink: using closed connection")
)

// connection represents a connection to generic netlink
type connection struct {
	mu     sync.Mutex
	sockFD int
	seq    uint32
	addr   syscall.SockaddrNetlink
	pid    uint32
	closed bool
}

func newConnection() (*connection, error) {
	sockFD, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_RAW|syscall.SOCK_CLOEXEC, syscall.NETLINK_GENERIC)
	if err != nil {
		return nil, err
	}

	c := &connection{
		sockFD: sockFD,
		seq:    rand.Uint32(),
		pid:    uint32(os.Getpid()),
		addr: syscall.SockaddrNetlink{
			Family: syscall.AF_NETLINK,
		},
	}

	if err := syscall.Bind(c.sockFD, &c.addr); err != nil {
		syscall.Close(c.sockFD)
		return nil, err
	}
	return c, nil
}

// doRequest sends and receives message with concurrent safe.
func (c *connection) doRequest(m syscall.NetlinkMessage) ([]syscall.NetlinkMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, ErrUsingClosedConn
	}

	reqM, err := c.sendMsg(m)
	if err != nil {
		return nil, err
	}
	return c.receive(reqM)
}

func (c *connection) close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.closed = true
	return syscall.Close(c.sockFD)
}

func (c *connection) sendMsg(m syscall.NetlinkMessage) (syscall.NetlinkMessage, error) {
	m.Header.Seq, c.seq = c.seq, c.seq+1

	if m.Header.Pid == 0 {
		m.Header.Pid = c.pid
	}
	return m, syscall.Sendto(c.sockFD, marshalMessage(m), 0, &c.addr)
}

func (c *connection) receive(reqM syscall.NetlinkMessage) ([]syscall.NetlinkMessage, error) {
	res := make([]syscall.NetlinkMessage, 0)

	for {
		parts, err := c.recvMsgs()
		if err != nil {
			return nil, err
		}

		isDone := true
		for _, p := range parts {
			if err := c.validateMessage(reqM, p); err != nil {
				return nil, err
			}

			if p.Header.Flags&syscall.NLM_F_MULTI == 0 {
				continue
			}

			isDone = (p.Header.Type == syscall.NLMSG_DONE)
		}

		res = append(res, parts...)
		if isDone {
			break
		}
	}

	if len(res) > 1 && res[len(res)-1].Header.Type == syscall.NLMSG_DONE {
		res = res[:len(res)-1]
	}

	if len(res) == 1 && res[0].Header.Type == syscall.NLMSG_DONE {
		return nil, ErrUnexpectedDoneMsg
	}
	return res, nil
}

func (c *connection) recvMsgs() ([]syscall.NetlinkMessage, error) {
	b := make([]byte, sysPageSize)

	for {
		n, _, _, _, err := syscall.Recvmsg(c.sockFD, b, nil, syscall.MSG_PEEK)
		if err != nil {
			return nil, err
		}

		// need more bytes to do align if equal
		if n < len(b) {
			break
		}
		b = make([]byte, len(b)+sysPageSize)
	}

	n, _, _, _, err := syscall.Recvmsg(c.sockFD, b, nil, 0)
	if err != nil {
		return nil, err
	}

	n = msgAlign(n)
	return syscall.ParseNetlinkMessage(b[:n])
}

func (c *connection) validateMessage(reqM, m syscall.NetlinkMessage) error {
	if m.Header.Flags != syscall.NLMSG_ERROR {
		if m.Header.Seq != reqM.Header.Seq {
			return ErrMismatchedSequence
		}

		if m.Header.Pid != reqM.Header.Pid {
			return ErrMismatchedPortID
		}
		return nil
	}

	// from https://www.infradead.org/~tgr/libnl/doc/core.html#core_errmsg
	//
	// Error messages can be sent in response to a request. Error messages
	// must use the standard message type NLMSG_ERROR. The payload consists
	// of a error code and the original netlink mesage header of the
	// request.
	//
	// The error code will take 32 bits in the payload.
	if len(m.Data) < 4 {
		return ErrInvalidErrorCode
	}

	errCode := getSysEndian().Uint32(m.Data[:4])
	return fmt.Errorf("genetlink: receive error code %v", syscall.Errno(errCode))
}
