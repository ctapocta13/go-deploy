package main

import (
	"flag"
	"fmt"
	// "os"
)

func main() {
	// fmt.Println("hello world!")
	// osArgs := os.Args
	// argsWOline := osArgs[1:]
	// fmt.Println(osArgs)
	// fmt.Println(argsWOline)

	cmdProject := flag.String("project", "", "Указываем папку проекта")
	cmdTask := flag.Int("task", 0, "Указываем номер задачи по проекту")

	flag.Parse()

	fmt.Println("project: ", *cmdProject)
	fmt.Println("task: ", *cmdTask)
}
