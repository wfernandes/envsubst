package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/drone/envsubst"
)

func main() {
	// stdin := bufio.NewScanner(os.Stdin)
	stdout := bufio.NewWriter(os.Stdout)

	fileBytesm, err := ioutil.ReadFile("/tmp/foo")
	if err != nil {
		log.Fatalf("Unable to read file")
	}
	line, err := envsubst.EvalEnv(string(fileBytesm))
	if err != nil {
		log.Fatalf("Error while envsubst: %v", err)
	}

	_, err = fmt.Fprintln(stdout, line)
	if err != nil {
		log.Fatalf("Error while writing to stdout: %v", err)
	}
	stdout.Flush()

	// for stdin.Scan() {
	// 	line, err := envsubst.EvalEnv(stdin.Text())
	// 	if err != nil {
	// 		log.Fatalf("Error while envsubst: %v", err)
	// 	}
	// 	_, err = fmt.Fprintln(stdout, line)
	// 	if err != nil {
	// 		log.Fatalf("Error while writing to stdout: %v", err)
	// 	}
	// 	stdout.Flush()
	// }
}
