// @@
// @ Author       : Eacher
// @ Date         : 2023-02-20 08:50:39
// @ LastEditTime : 2024-01-09 16:30:44
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
)

func main() {
	fmt.Println("Start")
	c, err := NewCan("can0")
	if err == nil {
		isotp.Init(c)
		if itp := isotp.IsoTP(128, 384); itp != nil {
			itp.ResetConfig(isotp.Config{STmin: 0x14, BS: 0x00, N_Re: 0xFF, N_Se: 0xFF}) // 20毫秒不限制接收帧数，再无流控帧发送
			go func() {
				b := itp.ReadData()
				for b != nil {
					fmt.Println(string(b))
					b = itp.ReadData()
					fmt.Println("128, 384", itp.WriteData([]byte(code)))
				}
			}()
		}
		if itp := isotp.IsoTP(1, 257); itp != nil {
			itp.ResetConfig(isotp.Config{STmin: 0x0A, BS: 0x0F, N_Re: 0xFF, N_Se: 0xFF}) // 10毫秒内每次能接收16帧，然后再发送一帧流控帧
			go func() {
				b := itp.ReadData()
				for b != nil {
					fmt.Println(string(b))
					b = itp.ReadData()
					fmt.Println("1, 257", itp.WriteData([]byte(`func (c *Can) WriteFrame(frame can.Frame) error {
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
