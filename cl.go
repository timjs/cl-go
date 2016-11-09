package main

import (
	"fmt"
	"log"
	"os"
)

const usage = `Clean command line tools

Usage:
    cl <command> [<args>...]

Available subcommands include:
    help
    init
    add, create
    remove, rm, delete
    move, mv
    switch
    build
    run
    clean
    prune

You can learn more about a specific command by running:
    cl <command> --help`

// In all these cases we are simply running 'cl-<command>' so if you create an
// executable named 'cl-foobar' you will be able to run it as 'cl foobar' as
// long as it appears on your PATH.`,

func quote(s string) string {
	return "'" + s + "'"
}

var (
	actionLog = log.New(os.Stdout, ">>> ", 0)
	errorLog  = log.New(os.Stderr, "!!! ", 0)
)

func runHelp() {
	fmt.Println(usage)
}

func runInit()   {}
func runAdd()    {}
func runRemove() {}
func runMove()   {}
func runSwitch() {}
func runBuild()  {}
func runRun()    {}
func runClean()  {}
func runPrune()  {}

func exitNoCommand() {
	runHelp()
	os.Exit(2)
}

func exitInvalidCommand(cmd string) {
	errorLog.Println(quote(cmd), "is not a valid subcommand")
	errorLog.Println("Run 'cl help' to see a list of all available subcommands")
	os.Exit(2)
}

func main() {

	if len(os.Args) == 1 {
		exitNoCommand()
	}

	switch os.Args[1] {
	case "help":
		runHelp()
	case "init":
		runInit()
	case "add", "create":
		runAdd()
	case "remove", "rm", "delete":
		runRemove()
	case "move", "mv":
		runMove()
	case "switch":
		runSwitch()
	case "build":
		runBuild()
	case "run":
		runRun()
	case "clean":
		runClean()
	case "prune":
		runPrune()
	default:
		exitInvalidCommand(os.Args[1])
	}
}
