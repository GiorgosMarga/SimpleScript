

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
	)

	flag.StringVar(&ipAddr, "ip", "127.0.0.0", "The ip address of the environment. Used is case of a migration")
	flag.StringVar(&port, "p", ":3000", "The port of the environment. Used is case of a migration")
	flag.Parse()

	e := environment.NewEnvironment(ipAddr, port)
	log.Fatal(e.Start())
}
