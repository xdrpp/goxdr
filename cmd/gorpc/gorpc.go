package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/xdrpp/goxdr/rpc"
)

func main() {
	filepath := flag.String("f", "", "Name of input file")
	flag.Parse()

	err := rpc.ReceiveFile(*filepath, func(x *rpc.Rpc_msg) {
		fmt.Println(MustXdrToJsonString(x))
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "receive file: %v\n", err)
		os.Exit(1)
	}
}
