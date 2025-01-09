# can-isotp
golang network can bus isotp protocol

# 简介

协议借鉴 [Linux Kernel Module for ISO 15765-2:2016](https://github.com/hartkopp/can-isotp.git)

## 例子

```go
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
```
