package main

// TODO
// - add/remove/move modules in config too
// - remove `os.Chdir`s
// - support for building standalone file (new and legacy)

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/bmatcuk/doublestar"
)

// Constants ///////////////////////////////////////////////////////////////////

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

const (
	headerPrefix   = ">> module "
	exportedPrefix = ">> "
	internalPrefix = ">  " //XXX be aware of the double spaces!!!
)

const (
	projectFileName       = "Project.toml"
	legacyProjectFileName = "Project.prj"
	legacyConfig          = `Version: 1.4
Global
	ProjectRoot:	.
	Target:	iTasks
	Exec:	{Project}/{{.Executable.Output}}
	CodeGen
		CheckStacks:	False
		CheckIndexes:	True
	Application
		HeapSize:	209715200
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
			Dynamics:	True
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
)

// Helpers /////////////////////////////////////////////////////////////////////

func expect(err error, msg ...string) {
	if err != nil {
		str := strings.Trim(fmt.Sprint(msg), "[]")
		errorLog.Fatalf("%s (%v)", str, err)
		// errorLog.Printn(msg...)
		// errorLog.Fatalln(err)
	}
}

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

// Manifest ////////////////////////////////////////////////////////////////////

//NOTE: Nested structs don't have a constructor, so we define them all seperately
type (
	Manifest struct {
		Project    ProjectInfo
		Executable ExecutableInfo
	}

	ProjectInfo struct {
		Name    string
		Version string
		Authors []string

		Sourcedir    string
		Modules      []string
		OtherModules []string
		Libraries    []string
	}

	ExecutableInfo struct {
		Main   string
		Output string
	}

	// Library struct {
	//     Exported []string
	//     Internal []string
	// }
)

var DefaultManifest = Manifest{
	Project: ProjectInfo{
		Name:    "application",
		Version: "0.0.0",

		Sourcedir: "src",
		Libraries: []string{
			"Dynamics",
			"Generics",
			"Platform",
		},
	},
	Executable: ExecutableInfo{
		Main: "Main",
	},
}

// Project /////////////////////////////////////////////////////////////////////

type Project struct {
	Manifest Manifest
}

func NewProject() Project {
	file, err := os.Open(projectFileName)
	defer file.Close()
	expect(err, "Could not find a project file, run 'cl init' to initialise a project")

	bytes, err := ioutil.ReadAll(file)
	expect(err, "Could not read project file")

	manifest := DefaultManifest
	expect(toml.Unmarshal(bytes, &manifest), "Could not parse project file")

	// Set defaults:
	if manifest.Executable.Output == "" {
		manifest.Executable.Output = manifest.Project.Name
	}

	return Project{manifest}
}

func InitProject() {
	actionLog.Println("Initializing new project")
	//FIXME: create project file

	os.Mkdir("src", 0755)
	os.Mkdir("test", 0755)
}

// Commands ////////////////////////////////////////////////////////////////////

func (prj *Project) Info() {
	actionLog.Println("Showing information about current project")

	expect(toml.NewEncoder(os.Stdout).Encode(prj.Manifest), "Could not encode project information")
}

func (prj *Project) Add(mods ...string) {
	os.Chdir(prj.Manifest.Project.Sourcedir)

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
	os.Chdir(prj.Manifest.Project.Sourcedir)

	for _, mod := range mods {
		actionLog.Println("Removing module", quote(mod))

		path := dotToSlash.Replace(mod)
		os.Remove(path + ".dcl")
		os.Remove(path + ".icl")
	}
}

func (prj *Project) Move(oldmod, newmod string) {
	actionLog.Println("Moving", quote(oldmod), "to", quote(newmod))

	os.Chdir(prj.Manifest.Project.Sourcedir)

	oldpath := dotToSlash.Replace(oldmod)
	newpath := dotToSlash.Replace(newmod)

	os.MkdirAll(filepath.Dir(newpath), 0755)
	os.Rename(oldpath+".dcl", newpath+".dcl")
	os.Rename(oldpath+".icl", newpath+".icl")
}

func (prj *Project) Unlit() {
	actionLog.Println("Unliterating modules")

	unlitHelper(prj.Manifest.Project.Sourcedir, prj.Manifest.Executable.Main)

	for _, mod := range prj.Manifest.Project.Modules {
		unlitHelper(prj.Manifest.Project.Sourcedir, mod)
	}
	for _, mod := range prj.Manifest.Project.OtherModules {
		unlitHelper(prj.Manifest.Project.Sourcedir, mod)
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
	expect(err, "Could not open", quote(path))
	ifile, err := os.Create(ipath)
	defer ifile.Close()
	expect(err, "Could not open", quote(path))
	dfile, err := os.Create(dpath)
	defer dfile.Close()
	expect(err, "Could not open", quote(path))

	scanner := bufio.NewScanner(lfile)
	iwriter := bufio.NewWriter(ifile)
	defer iwriter.Flush()
	dwriter := bufio.NewWriter(dfile)
	defer dwriter.Flush()
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, headerPrefix) {
			code := strings.TrimPrefix(line, exportedPrefix)
			fmt.Fprintln(iwriter, "implementation", code)
			fmt.Fprintln(dwriter, "definition", code)
		} else if strings.HasPrefix(line, exportedPrefix) {
			code := strings.TrimPrefix(line, exportedPrefix)
			fmt.Fprintln(iwriter, code)
			fmt.Fprintln(dwriter, code)
		} else if strings.HasPrefix(line, internalPrefix) {
			code := strings.TrimPrefix(line, internalPrefix)
			fmt.Fprintln(iwriter, code)
			fmt.Fprintln(dwriter)
		} else {
			fmt.Fprintln(iwriter)
			fmt.Fprintln(dwriter)
		}
	}
}

func (prj *Project) Build() {
	prj.Unlit()

	actionLog.Println("Building project")

	args := buildArgs(prj.Manifest, prj.Manifest.Executable.Main, "-o", prj.Manifest.Executable.Output)

	cmd := exec.Command("clm", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	expect(cmd.Run(), "Could not run 'clm'")
}

func (prj *Project) Run() {
	prj.Build()

	actionLog.Println("Running project")

	out := prj.Manifest.Executable.Output
	cmd := exec.Command("./" + out)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	//NOTE: `cmd.Run()` lets your ignore the error and silently fails if command could not be found...
	expect(cmd.Run(), "Could not run", quote(out))
}

func (prj *Project) List() {
	prj.Unlit()

	actionLog.Println("Collecting types of functions")

	args := buildArgs(prj.Manifest, "-lat", prj.Manifest.Executable.Main)

	cmd := exec.Command("clm", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	expect(cmd.Run(), "Could not run 'clm'")
}

func buildArgs(manifest Manifest, extra ...string) []string {
	args := make([]string, 0, 2*len(manifest.Project.Libraries)+len(extra)) // Reserve space for possible additional arguments
	args = append(args, "-I", manifest.Project.Sourcedir)
	for _, lib := range manifest.Project.Libraries {
		args = append(args, "-IL", lib)
	}
	args = append(args, extra...)
	return args
}

func (prj *Project) Clean() {
	actionLog.Println("Cleaning files")

	todo := make([]string, 0, 32)

	var glob []string
	glob, _ = doublestar.Glob("**/Clean System Files/")
	todo = append(todo, glob...)
	glob, _ = doublestar.Glob("*-sapl")
	todo = append(todo, glob...)
	glob, _ = doublestar.Glob("*-www")
	todo = append(todo, glob...)

	for _, path := range todo {
		//NOTE: Here we could also add a check if files exist, but Glob already does that.
		infoLog.Println(path)
		os.RemoveAll(path)
	}
}

func (prj *Project) Prune() {
	prj.Clean()

	actionLog.Println("Pruning files")

	todo := make([]string, 0, 3)

	var glob []string
	glob, _ = doublestar.Glob(prj.Manifest.Executable.Output)
	todo = append(todo, glob...)
	glob, _ = doublestar.Glob(legacyProjectFileName)
	todo = append(todo, glob...)
	glob, _ = doublestar.Glob("*-data")
	todo = append(todo, glob...)

	for _, path := range todo {
		infoLog.Println(path)
		os.RemoveAll(path)
	}
}

// Legacy commands /////////////////////////////////////////////////////////////

func (prj *Project) LegacyGen() {
	actionLog.Println("Generating legacy project configuration")

	temp := template.Must(template.New("legacy config").Parse(legacyConfig))
	out, err := os.Create(legacyProjectFileName)
	defer out.Close()
	expect(err, "Could not create", quote(legacyProjectFileName))

	expect(temp.Execute(out, prj.Manifest), "Error writing legacy configuration file")
}

func (prj *Project) LegacyBuild() {
	prj.LegacyGen()
	prj.Unlit()

	actionLog.Println("Building project")

	cmd := exec.Command("cpm", legacyProjectFileName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	expect(cmd.Run(), "Could not run 'cpm'")
}

func (prj *Project) LegacyRun() {
	prj.LegacyBuild()

	actionLog.Println("Running project")

	out := prj.Manifest.Executable.Output
	cmd := exec.Command("./" + out)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	expect(cmd.Run(), "Could not run", quote(out))
}

// Main ////////////////////////////////////////////////////////////////////////

func main() {

	if len(os.Args) == 1 {
		infoLog.Fatalln(usage)
	}

	switch os.Args[1] {
	case "help":
		infoLog.Println(usage)
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
