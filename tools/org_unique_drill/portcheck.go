//go:build ignore

package main

import (
	"fmt"
	"net"
	"time"
)

func main() {
	d := net.Dialer{Timeout: 2 * time.Second}
	c, err := d.Dial("tcp", "127.0.0.1:13306")
	if err != nil {
		fmt.Println("PORT_CLOSED")
		return
	}
	_ = c.Close()
	fmt.Println("PORT_OPEN")
}
