// @@
// @ Author       : Eacher
// @ Date         : 2023-02-20 08:50:39
// @ LastEditTime : 2025-01-09 09:04:08
// @ LastEditors  : Eacher
// @ --------------------------------------------------------------------------------<
// @ Description  : Linux can isotp protocol 使用例子
// @ --------------------------------------------------------------------------------<
// @ FilePath     : /20yyq/can-isotp/examples/main.go
// @@
package main

import (
	"errors"
	"fmt"

	"github.com/20yyq/can-debugger/sockcan"
	"github.com/20yyq/isotp"
	"github.com/20yyq/packet/can"
	"golang.org/x/sys/unix"
)

func main() {
	fmt.Println("Start")
	c, err := NewCan("can0")
	if err == nil {
		isotp.Init(c)
		// 每帧连续帧间隔20毫秒，不限制接收帧数，再无流控帧发送
		if itp := isotp.IsoTP(isotp.Config{ID: 128, STmin: 0x14, BS: 0x00}, isotp.Config{ID: 384, STmin: 0x14, BS: 0x00}); itp != nil {
			go func() {
				b := make([]byte, 4096)
				n, err := itp.Read(b)
				for err == nil {
					fmt.Println(string(b[:n]))
					n, err = itp.Read(b)
					fmt.Println(itp.Write([]byte(code)))
				}
			}()
		}
		// 每帧连续帧间隔10毫秒接收16帧，然后等待下一帧流控帧
		if itp := isotp.IsoTP(isotp.Config{ID: 1, STmin: 0x0A, BS: 0x0F}, isotp.Config{ID: 257, STmin: 0x0A, BS: 0x0F}); itp != nil {
			go func() {
				b := make([]byte, 4096)
				n, err := itp.Read(b)
				for err != nil {
					fmt.Println(string(b[:n]))
					n, err = itp.Read(b)
					fmt.Println(itp.Write([]byte(`func (c *Can) WriteFrame(frame can.Frame) error {
						_, err := c.rwc.Write(frame.WireFormat())
						return err
					}`)))
				}
			}()
		}

	}
	fmt.Println("End", err)
}

type Can struct {
	conn *sockcan.Can
}

func NewCan(dev string) (*Can, error) {
	conn, err := sockcan.NewCan(dev)
	if err == nil {
		return &Can{conn: conn}, nil
	}
	return nil, err
}

func (c *Can) ReadFrame() (can.Frame, error) {
	f, err := c.conn.ReadFrame()
	// TODO 做初始化CAN-ID等操作
	if false { // 没有初始化CAN-ID 不做协议处理
		return can.Frame{}, errors.New("not Frame")
	}
	return f, err
}

func (c *Can) WriteFrame(frame can.Frame) error {
	// TODO 完成初始化后再做处理帧
	if false {
		return errors.New("not Frame")
	}
	return c.conn.WriteFrame(frame)
}

func (c *Can) AddCanFilter(frame unix.CanFilter) error {
	// TODO
	return nil
}

const code = `
type Can struct {
	conn *sockcan.Can
}

var id uint32 = 128
var rxid uint32 = 384

// var id uint32 = 1
// var rxid uint32 = 257

func NewCan(dev string) (*Can, error) {
	conn, err := sockcan.NewCan(dev)
	if err == nil {
		return &Can{conn: conn}, nil
	}
	return nil, err
}

func (c *Can) ReadFrame() (can.Frame, error) {
	f, err := c.conn.ReadFrame()
	// TODO 做初始化ID等操作
	if false { // 没有初始化CAN-ID 不做协议处理
		return can.Frame{}, errors.New("not Frame")
	}
	return f, err
}

func (c *Can) WriteFrame(frame can.Frame) error {
	// TODO 完成初始化后再做处理帧
	if false {
		return errors.New("not Frame")
	}
	return c.conn.WriteFrame(frame)
}
`
