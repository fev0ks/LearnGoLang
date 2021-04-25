package logging

import (
	"fmt"
	"time"
)

//duplicate file to check what will be if there are the same packages are used...
var debug bool

func Debug(b bool) {
	debug = b
}

func Log(statement string) {
	if !debug {
		return
	}

	fmt.Printf("%s %s\n", time.Now().Format(time.RFC3339), statement)
}
