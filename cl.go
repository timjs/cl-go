package main

// TODO
// - add/remove/move modules in config too
// - remove `os.Chdir`s

import (
	"bufio"
	"bytes"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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
    info
    add, create
    remove, rm, delete
    move, mv
    unlit
    build
    run
    list
    clean
    prune`

// You can learn more about a specific command by running:
//     cl <command> --help`

// In all these cases we are simply running 'cl-<command>' so if you create an
// executable named 'cl-foobar' you will be able to run it as 'cl foobar' as
// long as it appears on your PATH.`,

// Helpers /////////////////////////////////////////////////////////////////////

func quote(s string) string {
	return "'" + s + "'"
}

var (
	actionLog  = log.New(os.Stdout, ">> ", 0)
	infoLog    = log.New(os.Stdout, ":: ", 0)
	warningLog = log.New(os.Stderr, "** ", 0)
	errorLog   = log.New(os.Stderr, "!! ", 0)
	debugLog   = log.New(os.Stderr, ".. ", 0)
)

var (
	slashToDot = strings.NewReplacer(string(os.PathSeparator), ".")
	dotToSlash = strings.NewReplacer(".", string(os.PathSeparator))
)

var (
	headerPrefix   = []byte(">> module ")
	exportedPrefix = []byte(">> ")
	internalPrefix = []byte(">  ") //XXX be aware of the double spaces!!!
)

// Config //////////////////////////////////////////////////////////////////////

type config struct {
	Project struct {
		Name    string
		Version string
		Authors []string

		Sourcedir    string
		Modules      []string
		OtherModules []string
		Libraries    []string
	}

	Executable struct {
		Main   string
		Output string
	}

	// Library struct {
	//     Exported []string
	//     Internal []string
	// }
}

func readProjectFile() config {
	file, err := os.Open(projectfile)
	defer file.Close()
	if err != nil {
		errorLog.Fatalln("Could not find a project file, run 'cl init' to initialise a project")
	}

	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		errorLog.Fatalln("Could not read project file", err)
	}

	var conf config
	if err := toml.Unmarshal(bytes, &conf); err != nil {
		errorLog.Fatalln("Could not parse project file", err)
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

		path := dotToSlash.Replace(mod)
		os.MkdirAll(filepath.Dir(path), 0755)

		dcl, _ := os.Create(path + ".dcl")
		defer dcl.Close()
		dcl.WriteString("definition module " + mod + "\n\n")

		ipath, _ := os.Create(path + ".icl")
		defer ipath.Close()
		ipath.WriteString("implementation module " + mod + "\n\n")
	}
}

func runRemove(conf config, mods ...string) {
	os.Chdir(conf.Project.Sourcedir)

	for _, mod := range mods {
		actionLog.Println("Removing module", quote(mod))

		path := dotToSlash.Replace(mod)
		os.Remove(path + ".dcl")
		os.Remove(path + ".icl")
	}
}

func runMove(conf config, oldmod, newmod string) {
	actionLog.Println("Moving", quote(oldmod), "to", quote(newmod))

	os.Chdir(conf.Project.Sourcedir)

	oldpath := dotToSlash.Replace(oldmod)
	newpath := dotToSlash.Replace(newmod)

	os.MkdirAll(filepath.Dir(newpath), 0755)
	os.Rename(oldpath+".dcl", newpath+".dcl")
	os.Rename(oldpath+".icl", newpath+".icl")
}

func runUnlit(conf config) {
	actionLog.Println("Unliterating modules")

	unlitHelper(conf.Project.Sourcedir, conf.Executable.Main)

	for _, mod := range conf.Project.Modules {
		unlitHelper(conf.Project.Sourcedir, mod)
	}
	for _, mod := range conf.Project.OtherModules {
		unlitHelper(conf.Project.Sourcedir, mod)
	}
}

func unlitHelper(dir string, mod string) {
	path := filepath.Join(dir, dotToSlash.Replace(mod))
	lpath := path + ".lcl"
	ipath := path + ".icl"
	dpath := path + ".dcl"

	lstat, err := os.Stat(lpath)
	if err != nil {
		// debugLog.Println("No literate file for", mod)
		return
	}
	mtime := lstat.ModTime()

	var itime, dtime time.Time
	istat, err := os.Stat(ipath)
	if err == nil {
		itime = istat.ModTime()
	}
	dstat, err := os.Stat(dpath)
	if err == nil {
		dtime = dstat.ModTime()
	}

	if mtime.Before(itime) && mtime.Before(dtime) {
		// debugLog.Println("Everything up-to-date for", mod)
		return
	}

	infoLog.Println(mod)

	lfile, err := os.Open(lpath) // lpath already exists...
	defer lfile.Close()
	if err != nil {
		errorLog.Fatalln("Could not open", quote(path), err)
	}
	ifile, err := os.Create(ipath)
	defer ifile.Close()
	if err != nil {
		errorLog.Fatalln("Could not open", quote(path), err)
	}
	dfile, err := os.Create(dpath)
	defer dfile.Close()
	if err != nil {
		errorLog.Fatalln("Could not open", quote(path), err)
	}

	scanner := bufio.NewScanner(lfile)
	iwriter := bufio.NewWriter(ifile)
	defer iwriter.Flush()
	dwriter := bufio.NewWriter(dfile)
	defer dwriter.Flush()
	for scanner.Scan() {
		line := scanner.Bytes()
		if bytes.HasPrefix(line, headerPrefix) {
			code := bytes.TrimPrefix(line, exportedPrefix)
			iwriter.WriteString("implementation ")
			iwriter.Write(code)
			iwriter.WriteString("\n")
			dwriter.WriteString("definition ")
			dwriter.Write(code)
			dwriter.WriteString("\n")
		} else if bytes.HasPrefix(line, []byte(exportedPrefix)) {
			code := bytes.TrimPrefix(line, exportedPrefix)
			iwriter.Write(code)
			iwriter.WriteString("\n")
			dwriter.Write(code)
			dwriter.WriteString("\n")
		} else if bytes.HasPrefix(line, []byte(internalPrefix)) {
			code := bytes.TrimPrefix(line, internalPrefix)
			iwriter.Write(code)
			iwriter.WriteString("\n")
			dwriter.WriteString("\n")
		} else {
			iwriter.WriteString("\n")
			dwriter.WriteString("\n")
		}
	}
}

func runBuild(conf config) {
	runUnlit(conf)

	actionLog.Println("Building project")

	args := buildArgs(conf, conf.Executable.Main, "-o", conf.Executable.Output)

	cmd := exec.Command("clm", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func runRun(conf config) { //FIXME
	runBuild(conf)

	actionLog.Println("Running project")

	cmd := exec.Command(conf.Executable.Output)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func runList(conf config) {
	actionLog.Println("Collecting types of functions")

	args := buildArgs(conf, "-lat", conf.Executable.Main)

	cmd := exec.Command("clm", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func buildArgs(conf config, extra ...string) []string {
	args := make([]string, 0, 2*len(conf.Project.Libraries)+len(extra)) // Reserve space for possible additional arguments
	args = append(args, "-I", conf.Project.Sourcedir)
	for _, lib := range conf.Project.Libraries {
		args = append(args, "-IL", lib)
	}
	args = append(args, extra...)
	return args
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

// Main ////////////////////////////////////////////////////////////////////////

func main() {

	if len(os.Args) == 1 {
		infoLog.Fatalln(usage)
	}

	switch os.Args[1] {
	case "help":
		runHelp()
	case "init":
		runInit()
	default:
		// For other options we need to be in a project directory
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
		case "unlit":
			runUnlit(conf)
		case "build":
			runBuild(conf)
		case "run":
			runRun(conf)
		case "list":
			runList(conf)
		case "clean":
			runClean(conf)
		case "prune":
			runPrune(conf)
		default:
			errorLog.Fatalln(quote(os.Args[1]), "is not a valid command, run 'cl help' to see a list of all available commands")
		}
	}
}
