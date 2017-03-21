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
	"text/template"
	"time"

	"github.com/naoina/toml"
)

// Constants ///////////////////////////////////////////////////////////////////

const PROJECT_FILE = "Project.toml"
const USAGE = `Clean command line tools

Usage:
    cl <command> [<args>...]

Available commands include:
    help
    init
    info
    add, create
    remove, rm, delete
    move, mv
    legacybuild, legacygen, legacyrun
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

const LEGACY_PROJECT_FILE = "Project.prj"
const LEGACY_CONFIG = `Version: 1.4
Global
	ProjectRoot:	.
	Target:	iTasks
	Exec:	{Project}/{{.Executable.Output}}
	CodeGen
		CheckStacks:	False
		CheckIndexes:	True
	Application
		HeapSize:	2097152
		StackSize:	512000
		ExtraMemory:	8192
		IntialHeapSize:	204800
		HeapSizeMultiplier:	4096
		ShowExecutionTime:	False
		ShowGC:	False
		ShowStackSize:	False
		MarkingCollector:	False
		DisableRTSFlags:	False
		StandardRuntimeEnv:	True
		Profile
			Memory:	False
			MemoryMinimumHeapSize:	0
			Time:	False
			Stack:	False
			Dynamics:	False
			DescExL:	False
		Output
			Output:	ShowConstructors
			Font:	Monaco
			FontSize:	9
			WriteStdErr:	False
	Link
		LinkMethod:	Static
		GenerateRelocations:	False
		GenerateSymbolTable:	False
		GenerateLinkMap:	False
		LinkResources:	False
		ResourceSource:
		GenerateDLL:	False
		ExportedNames:
	Paths
		Path:	{Project}/{{.Project.Sourcedir}}
	Precompile:
	Postlink:
MainModule
	Name:	{{.Executable.Main}}
	Dir:	{Project}/{{.Project.Sourcedir}}
	Compiler
		NeverMemoryProfile:	False
		NeverTimeProfile:	False
		StrictnessAnalysis:	True
		ListTypes:	StrictExportTypes
		ListAttributes:	True
		Warnings:	True
		Verbose:	True
		ReadableABC:	False
		ReuseUniqueNodes:	True
		Fusion:	False
`

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

type Config struct {
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

// Project /////////////////////////////////////////////////////////////////////

type Project struct {
	Config Config
}

func NewProject() Project {
	file, err := os.Open(PROJECT_FILE)
	defer file.Close()
	if err != nil {
		errorLog.Fatalln("Could not find a project file, run 'cl init' to initialise a project")
	}

	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		errorLog.Fatalln("Could not read project file", err)
	}

	var conf Config
	if err := toml.Unmarshal(bytes, &conf); err != nil {
		errorLog.Fatalln("Could not parse project file", err)
	}

	return Project{conf}
}

func InitProject() {
	actionLog.Println("Initializing new project")

	os.Mkdir("src", 0755)
	os.Mkdir("test", 0755)
}

// Commands ////////////////////////////////////////////////////////////////////

func (prj *Project) Info() {
	actionLog.Println("Showing information about current project")
	infoLog.Println(prj.Config)
}

func (prj *Project) Add(mods ...string) {
	os.Chdir(prj.Config.Project.Sourcedir)

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

func (prj *Project) Remove(mods ...string) {
	os.Chdir(prj.Config.Project.Sourcedir)

	for _, mod := range mods {
		actionLog.Println("Removing module", quote(mod))

		path := dotToSlash.Replace(mod)
		os.Remove(path + ".dcl")
		os.Remove(path + ".icl")
	}
}

func (prj *Project) Move(oldmod, newmod string) {
	actionLog.Println("Moving", quote(oldmod), "to", quote(newmod))

	os.Chdir(prj.Config.Project.Sourcedir)

	oldpath := dotToSlash.Replace(oldmod)
	newpath := dotToSlash.Replace(newmod)

	os.MkdirAll(filepath.Dir(newpath), 0755)
	os.Rename(oldpath+".dcl", newpath+".dcl")
	os.Rename(oldpath+".icl", newpath+".icl")
}

func (prj *Project) Unlit() {
	actionLog.Println("Unliterating modules")

	unlitHelper(prj.Config.Project.Sourcedir, prj.Config.Executable.Main)

	for _, mod := range prj.Config.Project.Modules {
		unlitHelper(prj.Config.Project.Sourcedir, mod)
	}
	for _, mod := range prj.Config.Project.OtherModules {
		unlitHelper(prj.Config.Project.Sourcedir, mod)
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

func (prj *Project) Build() {
	prj.Unlit()

	actionLog.Println("Building project")

	args := buildArgs(prj.Config, prj.Config.Executable.Main, "-o", prj.Config.Executable.Output)

	cmd := exec.Command("clm", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func (prj *Project) Run() {
	prj.Build()

	actionLog.Println("Running project")

	cmd := exec.Command("./" + prj.Config.Executable.Output)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	//NOTE: `cmd.Run()` lets your ignore the error and silently fails if command could not be found...
	if err := cmd.Run(); err != nil {
		errorLog.Fatalln(err)
	}
}

func (prj *Project) List() {
	prj.Unlit()

	actionLog.Println("Collecting types of functions")

	args := buildArgs(prj.Config, "-lat", prj.Config.Executable.Main)

	cmd := exec.Command("clm", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func buildArgs(conf Config, extra ...string) []string {
	args := make([]string, 0, 2*len(conf.Project.Libraries)+len(extra)) // Reserve space for possible additional arguments
	args = append(args, "-I", conf.Project.Sourcedir)
	for _, lib := range conf.Project.Libraries {
		args = append(args, "-IL", lib)
	}
	args = append(args, extra...)
	return args
}

func (prj *Project) Clean() {
	actionLog.Println("Cleaning files")

	filepath.Walk(".", func(path string, _ os.FileInfo, _ error) error {
		if base := filepath.Base(path); base == "Clean System Files" || base == "sapl" {
			infoLog.Println(path)
			os.RemoveAll(path)
		}
		return nil
	})
}

func (prj *Project) Prune() {
	prj.Clean()

	actionLog.Println("Pruning files")

	//NOTE: How ugly...
	todo := []string{"", "-data", "-sapl", "-www"}
	for _, name := range todo {
		path := prj.Config.Executable.Output + name
		infoLog.Println(path)
		os.RemoveAll(path)
	}

	infoLog.Println(LEGACY_PROJECT_FILE)
	os.RemoveAll(LEGACY_PROJECT_FILE)
}

// Legacy commands /////////////////////////////////////////////////////////////

func (prj *Project) LegacyGen() {
	actionLog.Println("Generating legacy project configuration")

	temp := template.Must(template.New("LEGACY_CONFIG").Parse(LEGACY_CONFIG))
	out, err := os.Create(LEGACY_PROJECT_FILE)
	defer out.Close()
	if err != nil {
		errorLog.Fatalln("Could not create", quote(LEGACY_PROJECT_FILE), err)
	}
	err = temp.Execute(out, prj.Config)
	if err != nil {
		errorLog.Fatalln("Error writing legacy configuration file", err)
	}
}

func (prj *Project) LegacyBuild() {
	prj.LegacyGen()
	prj.Unlit()

	actionLog.Println("Building project")

	cmd := exec.Command("cpm", "make")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func (prj *Project) LegacyRun() {
	prj.LegacyBuild()
	prj.Run()
}

// Main ////////////////////////////////////////////////////////////////////////

func main() {

	if len(os.Args) == 1 {
		infoLog.Fatalln(USAGE)
	}

	switch os.Args[1] {
	case "help":
		infoLog.Println(USAGE)
	case "init":
		InitProject()
	default:
		// For other options we need to be in a project directory
		prj := NewProject()

		switch os.Args[1] {
		case "info":
			prj.Info()
		case "add", "create":
			prj.Add(os.Args[2:]...)
		case "remove", "rm", "delete":
			prj.Remove(os.Args[2:]...)
		case "move", "mv":
			prj.Move(os.Args[2], os.Args[3])
		case "unlit":
			prj.Unlit()
		case "build":
			prj.Build()
		case "run":
			prj.Run()
		case "list":
			prj.List()
		case "clean":
			prj.Clean()
		case "prune":
			prj.Prune()
		case "legacygen":
			prj.LegacyGen()
		case "legacybuild":
			prj.LegacyBuild()
		case "legacyrun":
			prj.LegacyRun()
		default:
			errorLog.Fatalln(quote(os.Args[1]), "is not a valid command, run 'cl help' to see a list of all available commands")
		}
	}
}
