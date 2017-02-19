package main

import (
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/naoina/toml"
)

// Constants ///////////////////////////////////////////////////////////////////

const projectfile = "Project.toml"
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
	infoLog   = log.New(os.Stdout, "    ", 0)
	actionLog = log.New(os.Stdout, ">>> ", 0)
	errorLog  = log.New(os.Stderr, "!!! ", 0)
)

var (
	pathToModule = strings.NewReplacer("/", ".")
	moduleToPath = strings.NewReplacer(".", "/")
)

// Config //////////////////////////////////////////////////////////////////////

type config struct {
	Project struct {
		Name    string
		Version string
		Authors []string

		Sourcedir string
		Libraries []string
	}

	Executable struct {
		Main   string
		Output string
	}
}

func readProjectFile() config {
	// We already checked if this is a project!
	file, _ := os.Open(projectfile)
	defer file.Close()

	buf, _ := ioutil.ReadAll(file)

	var conf config
	if err := toml.Unmarshal(buf, &conf); err != nil {
		exitProjectParseError(err)
	}

	return conf
}

// Commands ////////////////////////////////////////////////////////////////////

func runHelp() {
	infoLog.Println(usage)
}

func runInit() {
	actionLog.Println("Initializing new project")

	os.Mkdir("src", 0755)
	os.Mkdir("test", 0755)
}

func runInfo(conf config) {
	actionLog.Println("Showing information about current project")
	infoLog.Println(conf)
}

func runAdd(conf config, mods ...string) {
	os.Chdir(conf.Project.Sourcedir)

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

func runRemove(conf config, mods ...string) {
	os.Chdir(conf.Project.Sourcedir)

	for _, mod := range mods {
		actionLog.Println("Removing module", quote(mod))

		path := moduleToPath.Replace(mod)
		os.Remove(path + ".dcl")
		os.Remove(path + ".icl")
	}
}

func runMove(conf config, oldmod, newmod string) {
	actionLog.Println("Moving", quote(oldmod), "to", quote(newmod))

	os.Chdir(conf.Project.Sourcedir)

	oldpath := moduleToPath.Replace(oldmod)
	newpath := moduleToPath.Replace(newmod)

	os.MkdirAll(filepath.Dir(newpath), 0755)
	os.Rename(oldpath+".dcl", newpath+".dcl")
	os.Rename(oldpath+".icl", newpath+".icl")
}

func runBuild(conf config, args ...string) {
	actionLog.Println("Building project")

	if len(args) > 0 {
		switch args[0] {
		case "--old", "--cpm":
			cmd := exec.Command("cpm", "make")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Run()
		}
	} else {
		os.Chdir(conf.Project.Sourcedir)

		args := make([]string, 0, 2*len(conf.Project.Libraries)+2)
		for _, lib := range conf.Project.Libraries {
			args = append(args, "-IL", lib)
		}
		args = append(args, conf.Executable.Main)
		args = append(args, "-o", conf.Executable.Output)

		cmd := exec.Command("clm", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
	}
}

func runRun(conf config) {
	actionLog.Println("Running project")

	cmd := exec.Command(conf.Executable.Main)
	cmd.Stdout = os.Stdout
	cmd.Run()
}

func runClean(conf config) {
	actionLog.Println("Cleaning files")

	filepath.Walk(".", func(path string, _ os.FileInfo, _ error) error {
		if base := filepath.Base(path); base == "Clean System Files" || base == "sapl" {
			infoLog.Println(path)
			os.RemoveAll(path)
		}
		return nil
	})
}

func runPrune(conf config) {
	runClean(conf)

	actionLog.Println("Pruning files")

	todo := make([]string, 0, 16)
	todo = append(todo, conf.Executable.Output)
	globs, _ := filepath.Glob("*-data/")
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
	os.Exit(1)
}

func exitInvalidCommand(cmd string) {
	errorLog.Println(quote(cmd), "is not a valid command")
	errorLog.Println("Run 'cl help' to see a list of all available commands")
	os.Exit(1)
}

func exitIfNotProject() {
	if _, err := os.Stat(projectfile); err != nil {
		errorLog.Println("This is not a Clean project directory")
		infoLog.Println("Run 'cl init' to initialise a project")
		os.Exit(2)
	}
}

func exitProjectParseError(err error) {
	errorLog.Println("Error parsing project file:")
	infoLog.Println(err)
	os.Exit(3)
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
	case "switch":
		runSwitch()
	default:
		// For other options we need to be in a project directory
		exitIfNotProject()
		conf := readProjectFile()

		switch os.Args[1] {
		case "info":
			runInfo(conf)
		case "add", "create":
			runAdd(conf, os.Args[2:]...)
		case "remove", "rm", "delete":
			runRemove(conf, os.Args[2:]...)
		case "move", "mv":
			runMove(conf, os.Args[2], os.Args[3])
		case "build":
			runBuild(conf, os.Args[2:]...)
		case "run":
			runRun(conf)
		case "clean":
			runClean(conf)
		case "prune":
			runPrune(conf)
		default:
			exitInvalidCommand(os.Args[1])
		}
	}
}
