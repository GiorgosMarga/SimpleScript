package main

import (
	"flag"
	"log"

	"github.com/GiorgosMarga/simplescript/environment"
)

func main() {

	var (
		ipAddr string
		port   string
		peers  string
	)

	flag.StringVar(&ipAddr, "ip", "127.0.0.0", "The ip address of the environment.")
	flag.StringVar(&port, "p", ":3000", "The port of the environment.")
	flag.StringVar(&peers, "peers", "", "The addresses of other environments.")
	flag.Parse()

	e := environment.NewEnvironment(ipAddr, port)
	log.Fatal(e.Start(peers))
}
