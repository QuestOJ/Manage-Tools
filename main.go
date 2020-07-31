package main

import (
	"flag"
	"fmt"
	"os"
)

const VERSION = "Manage tools version 3.2"

var worker bool
var dataDir string

func main() {
	fmt.Println(VERSION)

	flag.BoolVar(&worker, "worker", false, "use worker flag to run a task runner")
	flag.StringVar(&dataDir, "d", "./data", "data dir")
	flag.Parse()

	if worker == true {
		taskName = flag.Arg(0)
		taskID = flag.Arg(1)

		if len(flag.Args()) == 0 || taskName == "" {
			fmt.Println("Usage: tools [-d data dir] [task] [id] [arg1] [arg2] ... \n\nOptions:")
			flag.PrintDefaults()
			os.Exit(1)
		}

		Initworker()
	} else {
		Initserver()
	}
}
