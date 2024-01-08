// @@
// @ Author       : Eacher
// @ Date         : 2024-01-05 10:21:09
// @ LastEditTime : 2024-01-08 17:34:06
// @ LastEditors  : Eacher
// @ --------------------------------------------------------------------------------<
// @ Description  :
// @ --------------------------------------------------------------------------------<
// @ FilePath     : /20yyq/can-isotp/isotp.go
// @@
package isotp

import (
	"fmt"
	"io"
	"sync"
	"runtime"
	"time"

	"github.com/20yyq/packet/can"
)

const MAX_PACKET = 0xFFF // 单个协议包最大长度

const N_PCI_SF = 0x00       /* single frame */
const N_PCI_FF = 0x10       /* first frame */
const N_PCI_CF = 0x20       /* consecutive frame */
const N_PCI_FC_OVFLW = 0x32 /* flow control Overflow*/
const N_PCI_FC_CTS = 0x30   /* flow control Continue To Send*/
const N_PCI_FC_WT = 0x31    /* flow control Wait*/

const (
	ISOTP_IDLE          uint8 = iota // 空闲状态
	ISOTP_WAIT_FIRST_FC              // 等待流控状态
	ISOTP_WAIT_FC                    // 等待流控状态、超时等待
	ISOTP_WAIT_DATA                  // 发送首次流控帧后等待数据状态
	ISOTP_SENDING                    // 发送数据状态
)

var canConn = struct {
	mutex sync.RWMutex
	can   Can
	isotp map[uint32]*isoTP
}{
	isotp: map[uint32]*isoTP{},
}

type Can interface {
	WriteFrame(can.Frame) error
	ReadFrame() (can.Frame, error)
}

// 初始化CAN总线数据帧接口
func Init(c Can) {
	canConn.mutex.Lock()
	defer canConn.mutex.Unlock()
	if canConn.can == nil {
		canConn.can = c
		go func() {
			frame, err := canConn.can.ReadFrame()
			for err != io.EOF {
				if err == nil {
					go listener(frame)
				}
				frame, err = canConn.can.ReadFrame()
			}
		}()
	}
}

func listener(frame can.Frame) {
	var run func(can.Frame)
	pci, id := frame.Data[0]&0xF0, frame.ID()
	canConn.mutex.RLock()
	defer canConn.mutex.RUnlock()
	itp := canConn.isotp[id]
	switch pci {
	case N_PCI_SF, N_PCI_FF, N_PCI_CF:
		for _, v := range canConn.isotp {
			v.mutex.RLock()
			c := v.conn[id]
			v.mutex.RUnlock()
			if c != nil {
				switch pci {
				case N_PCI_SF:
					run = (&c.read).single
				case N_PCI_FF:
					run = (&c.read).first
				case N_PCI_CF:
					run = (&c.read).consecutive
				}
				break
			}
		}
		if run != nil {
			break
		}
		fallthrough
	case N_PCI_FC_CTS:
		if itp != nil {
			run = itp.flowFrame
			break
		}
		fallthrough
	default:
		run = ignoreFrame
	}
	go run(frame)
}

func send(f can.Frame) error {
	if canConn.can != nil {
		return canConn.can.WriteFrame(f)
	}
	return fmt.Errorf("not can")
}

func ignoreFrame(f can.Frame) {
	fmt.Println("N_PCI---------ignoreFrame----------N_PCI", f)
}

type isoTP struct {
	mutex sync.RWMutex
	txid  uint32
	conn  map[uint32]*Conn
}

func (itp *isoTP) flowFrame(f can.Frame) {
	var is bool
	itp.mutex.RLock()
	for _, c := range itp.conn {
		var run func(can.Frame)
		switch f.Data[0] {
		case N_PCI_FC_CTS:
			c.write.mutex.RLock()
			if c.write.state == ISOTP_WAIT_FIRST_FC || c.write.state == ISOTP_WAIT_FC {
				if c.write.state == ISOTP_WAIT_FIRST_FC {
					c.write.timer.Reset(0)
				}
				run = (&c.write).cts
			}
			c.write.mutex.RUnlock()
		case N_PCI_FC_WT:
			run = (&c.write).wait
		case N_PCI_FC_OVFLW:
			run = (&c.write).overflow
		}
		if run != nil {
			is = true
			go run(f)
		}
	}
	itp.mutex.RUnlock()
	if !is {
		go ignoreFrame(f)
	}
	// fmt.Println("---------------(itp *isoTP) checkFrame---------------")
	// fmt.Println(f)
	// fmt.Println("---------------(itp *isoTP) checkFrame---------------")
}

// txid 本地监听ID rxid 目标ID
func IsoTP(txid, rxid uint32) *Conn {
	canConn.mutex.Lock()
	defer canConn.mutex.Unlock()
	itp := canConn.isotp[txid]
	if itp == nil {
		itp = &isoTP{txid: txid, conn: map[uint32]*Conn{}}
		canConn.isotp[txid] = itp
	}
	itp.mutex.Lock()
	defer itp.mutex.Unlock()
	c := itp.conn[rxid]
	if c == nil {
		c = &Conn{parent: itp}
		c.read.cfg, c.read.c, c.read.timer = defaultConfig, make(chan []byte, 5), time.NewTimer(time.Hour*24*365)
		c.read.ff.Len, c.read.ff.Data = 3, [64]byte{N_PCI_FC_CTS, defaultConfig.BS, defaultConfig.STmin}
		c.r, c.write.rxid, c.write.cfg, c.write.timer = c.read.c, rxid, defaultConfig, time.NewTimer(time.Hour*24*365)
		(&c.read.ff).SetID(itp.txid)
		itp.conn[rxid] = c
	}
	return c
}

var defaultConfig = Config{
	STmin: 0x05,
	BS:    0x0F,
	N_Re:  0x7F,
	N_Se:  0x7F,
	dlc:   8,
}

type Config struct {
	STmin byte // 最小间隔时间（STmin，8bit）
	BS    byte // 块大小（BS，8bit）最大为0x0F 0x00 表示再无流控帧
	ISFD  bool
	N_Re  byte // 协议包最长接收时间（毫秒）
	N_Se  byte // 协议包最长发送时间（毫秒）
	dlc   uint8
}

type Conn struct {
	parent *isoTP
	read   read
	write  write
	r      <-chan []byte
}

func (c *Conn) ReadData() []byte {
	b, _ := <-c.r
	return b
}

func (c *Conn) WriteData(b []byte) error {
	c.write.mutex.Lock()
	defer c.write.mutex.Unlock()
	if c.write.state != ISOTP_IDLE {
		fmt.Println("---------------WriteData errors---------------")
		return fmt.Errorf("busy")
	}
	if c.write.len = uint16(len(b)); c.write.len > MAX_PACKET {
		return fmt.Errorf("too lenght")
	}
	c.write.n, c.write.sn = int(c.write.cfg.dlc-2), 1
	frame := &can.Frame{Len: uint8(c.write.len + 1)}
	frame.SetID(c.write.rxid)
	if int(c.write.len) > c.write.n {
		c.write.b, c.write.state = make([]byte, c.write.len), ISOTP_WAIT_FIRST_FC
		frame.Len, frame.Data[0], frame.Data[1] = c.write.cfg.dlc, byte(c.write.len>>8)|N_PCI_FF, byte(c.write.len)
		copy(c.write.b, b)
		copy(frame.Data[2:], c.write.b[:c.write.n])
	} else {
		frame.Data[0] = byte(c.write.len) | N_PCI_SF
		copy(frame.Data[1:], b)
	}
	c.write.timer.Reset(time.Millisecond * time.Duration(c.write.cfg.N_Se))
	go func(){
		send(*frame)
		start := time.Now()
		<-c.write.timer.C
		c.write.mutex.Lock()
		defer c.write.mutex.Unlock()
		if c.write.state == ISOTP_WAIT_FIRST_FC {
			c.write.state = ISOTP_IDLE
			fmt.Println("---------------WriteData time out---------------")
			return
		}
		c.write.state = ISOTP_WAIT_FC
		fmt.Println("---------------WriteData---------------")
		fmt.Println(runtime.NumGoroutine(), time.Now().Sub(start).Milliseconds())
	}()
	return nil
}

func (c *Conn) ResetConfig(cfg Config) error {
	c.write.mutex.Lock()
	defer c.write.mutex.Unlock()
	c.read.mutex.Lock()
	defer c.read.mutex.Unlock()
	if c.read.state != ISOTP_IDLE || c.write.state != ISOTP_IDLE {
		return fmt.Errorf("busy")
	}
	if cfg.N_Re < 1 {
		cfg.N_Re = 1
	}
	if cfg.N_Se < 1 {
		cfg.N_Se = 1
	}
	if cfg.dlc = 8; cfg.ISFD {
		cfg.dlc = 64
	}
	c.write.cfg = cfg
	c.read.cfg = cfg
	return nil
}
