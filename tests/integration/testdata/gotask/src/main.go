package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("Hello from mkGoTask!")
	fmt.Fprintf(os.Stderr, "Build successful\n")
	os.Exit(0)
}
