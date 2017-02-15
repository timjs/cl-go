package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Constants ///////////////////////////////////////////////////////////////////

const usage = `Clean command line tools

Usage:
    cl <command> [<args>...]

Available commands include:
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

// Helpers /////////////////////////////////////////////////////////////////////

func quote(s string) string {
	return "'" + s + "'"
}

var (
	infoLog   = log.New(os.Stdout, "", 0)
	actionLog = log.New(os.Stdout, ">>> ", 0)
	errorLog  = log.New(os.Stderr, "!!! ", 0)
)

var (
	pathToModule = strings.NewReplacer("/", ".")
	moduleToPath = strings.NewReplacer(".", "/")
)

// Commands ////////////////////////////////////////////////////////////////////

func runHelp() {
	infoLog.Println(usage)
}

func runInit() {
	actionLog.Println("Initializing new project")

	os.Mkdir("Sources", 0755)
	os.Mkdir("Tests", 0755)
}

func runAdd(mods ...string) {
	exitIfNotProject()
	os.Chdir("Sources")

	for _, mod := range mods {
		actionLog.Println("Creating module", quote(mod))

		path := moduleToPath.Replace(mod)
		os.MkdirAll(filepath.Dir(path), 0755)

		dcl, _ := os.Create(path + ".dcl")
		defer dcl.Close()
		dcl.WriteString("definition module " + mod + "\n\n")

		icl, _ := os.Create(path + ".icl")
		defer icl.Close()
		icl.WriteString("implementation module " + mod + "\n\n")
	}
}

func runRemove(mods ...string) {
	exitIfNotProject()
	os.Chdir("Sources")

	for _, mod := range mods {
		actionLog.Println("Removing module", quote(mod))

		path := moduleToPath.Replace(mod)
		os.Remove(path + ".dcl")
		os.Remove(path + ".icl")
	}
}

func runMove(oldmod, newmod string) {
	exitIfNotProject()
	actionLog.Println("Moving", quote(oldmod), "to", quote(newmod))

	os.Chdir("Sources")

	oldpath := moduleToPath.Replace(oldmod)
	newpath := moduleToPath.Replace(newmod)

	os.MkdirAll(filepath.Dir(newpath), 0755)
	os.Rename(oldpath+".dcl", newpath+".dcl")
	os.Rename(oldpath+".icl", newpath+".icl")
}

func runBuild() {
	exitIfNotProject()
	errorLog.Println("Not yet implemented")
}

func runRun() {
	exitIfNotProject()
	errorLog.Println("Not yet implemented")
}

func runClean() {
	exitIfNotProject()
	actionLog.Println("Cleaning files")

	filepath.Walk(".", func(path string, _ os.FileInfo, _ error) error {
		if base := filepath.Base(path); base == "Clean System Files" || base == "sapl" {
			infoLog.Println(path)
			os.RemoveAll(path)
		}
		return nil
	})
}

func runPrune() {
	runClean()

	actionLog.Println("Pruning files")

	todo := make([]string, 0, 16)
	globs, _ := filepath.Glob("*.exe")
	todo = append(todo, globs...)
	globs, _ = filepath.Glob("*-data/")
	todo = append(todo, globs...)

	for _, f := range todo {
		infoLog.Println(f)
		os.Remove(f)
	}
}

func runSwitch() {
	errorLog.Println("Not yet implemented")
}

// Exits ///////////////////////////////////////////////////////////////////////

func exitNoCommand() {
	infoLog.Println(usage)
	os.Exit(2)
}

func exitInvalidCommand(cmd string) {
	errorLog.Println(quote(cmd), "is not a valid command")
	errorLog.Println("Run 'cl help' to see a list of all available commands")
	os.Exit(2)
}

func exitIfNotProject() {
	if _, err := os.Stat("main.prj"); err != nil {
		errorLog.Println("This is not a Clean project directory")
		os.Exit(1)
	}
}

// Main ////////////////////////////////////////////////////////////////////////

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
		exitIfNotProject()
		runAdd(os.Args[2:]...)
	case "remove", "rm", "delete":
		exitIfNotProject()
		runRemove(os.Args[2:]...)
	case "move", "mv":
		exitIfNotProject()
		runMove(os.Args[2], os.Args[3])
	case "switch":
		exitIfNotProject()
	case "build":
		exitIfNotProject()
		runBuild()
	case "run":
		exitIfNotProject()
		runRun()
	case "clean":
		exitIfNotProject()
		runClean()
	case "prune":
		exitIfNotProject()
		runPrune()
	default:
		exitInvalidCommand(os.Args[1])
	}
}
