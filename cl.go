package main

// TODO
// - add/remove/move modules in config too
// - remove `os.Chdir`s
// - support for building standalone file (new and legacy)

import (
	"bufio"
	"fmt"
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
    cl <command> [<arguments>...]

Commands:
    help                Show this message
    init                Initialise new project
    show info           Show project info
    show types          Show types of all functions
    unlit               Unliterate modules
    build               Compile project
    run                 Build and run project
    clean               Clean build files
    prune               Clean and remove artifacts

Legacy commands:
    add, create         Add new instance and definition modules
    remove, rm, delete  Delete instance and definition modules
    move, mv            Move instance and definition modules
    generate            Generate legacy .prj file
    legacybuild         Build using legacy .prj file
    legacyrun           Build and run using legacy .prj file
`

// You can learn more about a specific command by running:
//     cl <command> --help`

// In all these cases we are simply running `cl-<command>` so if you create an
// executable named `cl-foobar` you will be able to run it as `cl foobar` as
// long as it appears on your PATH.`,

const (
	headerPrefix   = ">> module "
	exportedPrefix = ">> "
	internalPrefix = ">  " //XXX be aware of the double spaces!!!
)

const (
	projectFileName       = "Project.toml"
	legacyProjectFileName = "Project.prj"
	legacyConfigTemplate  = `Version: 1.4
Global
	ProjectRoot:	.
	Target:	iTasks
	Exec:	{Project}/{{.Executable.Name}}
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
		ListTypes:	AllTypes
		ListAttributes:	True
		Warnings:	True
		Verbose:	True
		ReadableABC:	False
		ReuseUniqueNodes:	True
		Fusion:	False
`
	mainTemplate = `module Main

import StdEnv

Start = "Hello World!"
`
)

// Helpers /////////////////////////////////////////////////////////////////////

func expect(err error, msg ...string) {
	if err != nil {
		str := strings.Trim(fmt.Sprint(msg), "[]")
		errorLog.Fatalf("%s\n   (%v)", str, err)
	}
}

func quote(s string) string {
	return "`" + s + "`"
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
		// Dependencies map[string]string //map[name]version
		// Executables []ExecutableInfo
		// Libraries []LibraryInfo
	}

	ProjectInfo struct {
		Name    string
		Version string
		Authors []string

		Sourcedir string `toml:",omitempty"` //Default: "src"

		Modules      []string //FIXME: Should move to LibraryInfo
		OtherModules []string //FIXME: Should move to LibraryInfo
		Libraries    []string //FIXME: Should move to Dependencies
	}

	ExecutableInfo struct {
		Name string `toml:",omitempty"` //Default: Project.Name
		Main string `toml:",omitempty"` //Default: "Main"
		// OtherModules []string //InternalModules?
	}

	// LibraryInfo struct {
	//     Name string
	//     ExposedModules []string
	//     OtherModules []string //InternalModules?
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
	actionLog.Println("Reading project file")

	file, err := os.Open(projectFileName)
	defer file.Close()
	expect(err, "Could not find a project file, run `cl init` to initialise a project")

	manifest := DefaultManifest
	md, err := toml.DecodeReader(file, &manifest)
	expect(err, "Could not parse project file")

	if keys := md.Undecoded(); len(keys) > 0 {
		warningLog.Println("Found undecoded keys, please update your project file:", keys)
	}

	// Set defaults:
	if manifest.Executable.Name == "" {
		manifest.Executable.Name = manifest.Project.Name
	}

	return Project{manifest}
}

func InitProject() {
	actionLog.Println("Initializing new project")

	file, err := os.Create(projectFileName)
	defer file.Close()
	expect(err, "Could not create project file")

	dir, err := os.Getwd()
	expect(err, "Could not get current directory name")

	mfst := Manifest{}
	mfst.Project.Name = filepath.Base(dir)
	mfst.Project.Version = "0.1.0"
	mfst.Project.Authors = []string{}

	enc := toml.NewEncoder(file)
	enc.Indent = ""
	expect(enc.Encode(mfst), "Could not encode project information")

	os.Mkdir("src", 0755)
	os.Mkdir("test", 0755)

	main, err := os.Create(filepath.Join("src", "Main.icl"))
	defer main.Close()
	expect(err, "Could not create main file")

	fmt.Fprintln(main, mainTemplate)
}

// Commands ////////////////////////////////////////////////////////////////////

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

	args := buildArgs(prj.Manifest, prj.Manifest.Executable.Main, "-o", prj.Manifest.Executable.Name)

	cmd := exec.Command("clm", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	expect(cmd.Run(), "`clm` ended abnormally")
}

func (prj *Project) Run() {
	prj.Build()

	actionLog.Println("Running project")

	out := prj.Manifest.Executable.Name
	cmd := exec.Command("./" + out)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	//NOTE: `cmd.Run()` lets your ignore the error and silently fails if command could not be found...
	expect(cmd.Run(), quote(out), "ended abnormally")
}

func (prj *Project) ShowInfo() {
	actionLog.Println("Showing information about current project")

	expect(toml.NewEncoder(os.Stdout).Encode(prj.Manifest), "Could not encode project information")
}

func (prj *Project) ShowTypes() {
	prj.Unlit()

	actionLog.Println("Collecting types of functions")

	now := time.Now()
	for _, name := range prj.Manifest.Project.Modules {
		path := filepath.Join(prj.Manifest.Project.Sourcedir, dotToSlash.Replace(name)) + ".icl"
		if err := os.Chtimes(path, now, now); err != nil {
			warningLog.Println("Could not touch", path)
		}
	}

	args := buildArgs(prj.Manifest, "-lat", prj.Manifest.Executable.Main)

	cmd := exec.Command("clm", args...)
	debugLog.Println(cmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	expect(cmd.Run(), "`clm` ended abnormally")
}

func buildArgs(manifest Manifest, extra ...string) []string {
	args := make([]string, 0, 2*len(manifest.Project.Libraries)+len(extra)) // Reserve space for possible additional arguments
	args = append(args, "-dynamics")
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
	glob, _ = doublestar.Glob(prj.Manifest.Executable.Name)
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

	temp := template.Must(template.New("legacy config").Parse(legacyConfigTemplate))
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
	expect(cmd.Run(), "`cpm` ended abnormally")
}

func (prj *Project) LegacyRun() {
	prj.LegacyBuild()

	actionLog.Println("Running project")

	out := prj.Manifest.Executable.Name
	cmd := exec.Command("./" + out)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	expect(cmd.Run(), quote(out), "ended abnormally")
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
		case "show":
			if len(os.Args) == 3 {
				switch os.Args[2] {
				case "info":
					prj.ShowInfo()
				case "types":
					prj.ShowTypes()
				}
			} else {
				prj.ShowInfo()
			}
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
		case "clean":
			prj.Clean()
		case "prune":
			prj.Prune()
		case "generate":
			prj.LegacyGen()
		case "legacybuild":
			prj.LegacyBuild()
		case "legacyrun":
			prj.LegacyRun()
		default:
			errorLog.Fatalln(quote(os.Args[1]), "is not a valid command, run `cl help` to see a list of all available commands")
		}
	}
}
