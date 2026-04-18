package main

import (
	"fmt"
	"net"
)

func main() {
	fmt.Println(net.JoinHostPort("::", "51322"))
    fmt.Println(net.JoinHostPort("0.0.0.0", "51322"))
    fmt.Println(net.JoinHostPort("[::]", "51322"))
}
