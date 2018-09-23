package main

import (
	"./mp"
	"fmt"
	"os"
	"os/signal"
)

func main() {
	mp.Start(":9998")
	fmt.Println("Race server running on port 9998")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
}
