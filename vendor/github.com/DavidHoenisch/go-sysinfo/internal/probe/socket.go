package probe

import "net"

type SocketConn interface {
	Write([]byte) (int, error)
	Read([]byte) (int, error)
	Close() error
}

type SocketDialer interface {
	Dial(network, address string) (SocketConn, error)
}

type OSSocketDialer struct{}

func (OSSocketDialer) Dial(network, address string) (SocketConn, error) {
	return net.Dial(network, address)
}

type MockSocketConn struct {
	WriteFn func([]byte) (int, error)
	ReadFn  func([]byte) (int, error)
	CloseFn func() error
}

func (c *MockSocketConn) Write(b []byte) (int, error) {
	if c.WriteFn != nil {
		return c.WriteFn(b)
	}
	return len(b), nil
}

func (c *MockSocketConn) Read(b []byte) (int, error) {
	if c.ReadFn != nil {
		return c.ReadFn(b)
	}
	return 0, nil
}

func (c *MockSocketConn) Close() error {
	if c.CloseFn != nil {
		return c.CloseFn()
	}
	return nil
}

type MockSocketDialer struct {
	DialFn func(network, address string) (SocketConn, error)
}

func (d MockSocketDialer) Dial(network, address string) (SocketConn, error) {
	if d.DialFn != nil {
		return d.DialFn(network, address)
	}
	return &MockSocketConn{}, nil
}
