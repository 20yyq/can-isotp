# can-isotp
golang network can bus isotp protocol
## 例子

```go
func main() {
	fmt.Println("Start")
	c, err := NewCan("can0")
	if err == nil {
		isotp.Init(c)
		if itp := isotp.IsoTP(128, 384); itp != nil {
			itp.Config = isotp.Config{STmin: 0x14, BS: 0x00} // 20毫秒不限制接收帧数，再无流控帧发送
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
			itp.Config = isotp.Config{STmin: 0x0A, BS: 0x0F} // 每10毫秒内接收16帧，然后再发送一帧流控帧
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
```
