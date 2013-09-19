// GoEconGo project main.go
package main

import (
	"fmt"
	"runtime"
)

func main() {
	fmt.Println("Hello World!")
}

//Set up our agent system/world state in here.
func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	fmt.Printf("Number of CPUS: %d\n", runtime.NumCPU())
}
