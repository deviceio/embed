package main

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

var flagLocal = flag.Bool("local", false, `Forces the generated code into local mode by default but will still embed files (see -embed). If you wish to change the mode you will need to either edit the embed file directly or re-run the embed cli command ommiting this flag`)
var flagEmbed = flag.Bool("embed", true, `Indicates if the generated code will contain the embeded file data or not. Setting -embed=false and -local=true permits will proxy all embeded file requests to the local filesystem and ensure fast code generation for development`)
var flagSilent = flag.Bool("silent", false, `Indicates if we wish to recieved output of the embedding operation`)

var dir = ""
var file = ""
var pkg = ""

func main() {
	flag.Parse()

	dir, err := filepath.Abs(filepath.Dir(os.Args[1]))
	dir = strings.Replace(dir, "\\", "/", -1)

	if err != nil {
		panic(err)
	}

	file = strings.Replace(filepath.Base(os.Args[1]), "\\", "/", -1)
	pkg = filepath.Base(dir)

	logPrintln(fmt.Sprintf("Generating embed file: %v/%v", dir, file))
	logPrintln(fmt.Sprintf("Generating in package: %v", pkg))
	logPrintln(fmt.Sprintf("Flags: -local=%v -embed=%v -silent=%v", *flagLocal, *flagEmbed, *flagSilent))
	logPrintln("Embedding Start:")

	fpath := fmt.Sprintf("%v/%v", dir, file)

	ioutil.WriteFile(
		fpath,
		mkTemplateBytes(codeTemplate, &codeTemplateVars{
			FlagLocal:  *flagLocal,
			FlagEmbed:  *flagEmbed,
			FlagSilent: *flagSilent,
			Package:    pkg,
			RootDir:    dir,
		}),
		os.ModeAppend,
	)

	f, err := os.OpenFile(fpath, os.O_APPEND, 0666)

	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	totalfiles := 0
	totalsize := int64(0)
	totalcomp := int64(0)
	startts := time.Now()

	if err != nil {
		log.Fatal(err)
	}

	if *flagEmbed {
		filepath.Walk(dir, func(fpath string, info os.FileInfo, err error) error {
			fpath = strings.Replace(fpath, "\\", "/", -1)

			logPrintln("Embedding:", fpath)

			if !info.IsDir() {
				totalsize = totalsize + info.Size()
			}

			if err != nil {
				return err
			}

			if info.Name() == file {
				return nil
			}

			finalPath := strings.Replace(fpath, dir, "/", -1)
			finalPath = path.Clean(finalPath)

			var data bytes.Buffer

			if !info.IsDir() {
				fh, err := os.Open(fpath)

				if err != nil {
					fh.Close()
					log.Fatal(err)
				}

				marshalGzipBase64(&data, fh)
				fh.Close()

				totalcomp = totalcomp + int64(len(data.Bytes()))
			}

			if _, err = f.Write(
				mkTemplateBytes(filemapValueTemplate, &filemapValueTemplateVars{
					FilePath:  finalPath,
					FileName:  info.Name(),
					FileSize:  info.Size(),
					FileMode:  uint32(info.Mode()),
					FileData:  string(data.Bytes()),
					FileIsDir: info.IsDir(),
				}),
			); err != nil {
				log.Fatal(err)
			}

			totalfiles = totalfiles + 1

			return err
		})
	}
	f.Write([]byte(filemapEndTemplate))

	statmb := float32(totalsize) / 1024 / 1024
	statmbs := float64(totalsize) / time.Since(startts).Seconds() / 1024 / 1024
	statcmpsaved := float32(totalsize-totalcomp) / 1024 / 1024
	statcmppct := statcmpsaved / statmb * 100
	statembmb := statmb - statcmpsaved

	logPrintln("Done:")
	logPrintln("Files Total: ", totalfiles)
	logPrintln("Files Total Size:", fmt.Sprintf("%.2f", statmb), "MB", fmt.Sprintf("%.2f", statmb*1024), "KB")
	logPrintln("Embeded Total Size:", fmt.Sprintf("%.2f", statembmb), "MB", fmt.Sprintf("%.2f", statembmb*1024), "KB")
	logPrintln("Read/Compress Bandwidth:", fmt.Sprintf("%.2f", statmbs), "MB/s", fmt.Sprintf("%.2f", statmbs*1024), "KB/s")
	logPrintln("Compression Savings:", fmt.Sprintf("%.2f", statcmpsaved), "MB", fmt.Sprintf("%.2f", statcmpsaved*1024), "KB")
	logPrintln("Compression Ratio:", fmt.Sprintf("%.2f", statcmppct), "%")
	logPrintln("Took:", time.Since(startts))
}

func logPrintln(v ...interface{}) {
	if !*flagSilent {
		log.Println(v...)
	}
}

func marshalGzipBase64(writer io.Writer, reader io.Reader) {
	b64 := base64.NewEncoder(base64.StdEncoding, writer)
	gz := gzip.NewWriter(b64)

	if _, err := io.Copy(gz, reader); err != nil {
		log.Fatal(err)
	}

	gz.Close()
	b64.Close()
}

func mkTemplateBytes(tmplstr string, vars interface{}) []byte {
	var outb = &bytes.Buffer{}

	tmpl, err := template.New("whatever").Parse(tmplstr)

	if err != nil {
		log.Fatal(err)
	}

	tmpl.Execute(outb, vars)

	return outb.Bytes()
}

type codeTemplateVars struct {
	FlagLocal  bool
	FlagEmbed  bool
	FlagSilent bool
	Package    string
	ModeLocal  bool
	ModeEmbed  bool
	RootDir    string
}

type filemapValueTemplateVars struct {
	FilePath  string
	FileSize  int64
	FileMode  uint32
	FileData  string
	FileName  string
	FileIsDir bool
}

var filemapValueTemplate = `"{{.FilePath}}": &embedFileInfo{     
    data: []byte("{{.FileData}}"),
    name: "{{.FileName}}",    
    isDir: {{.FileIsDir}},
    size: {{.FileSize}},
    mode: {{.FileMode}},
	mu: &sync.Mutex{},
},
`

var filemapEndTemplate = `}`

var codeTemplate = `//go:generate embed -local={{.FlagLocal}} -embed={{.FlagEmbed}} -silent={{.FlagSilent}}

package {{.Package}}

import (    
	"fmt"
	"os"
	"time"
    "bytes"   
    "compress/gzip"
    "encoding/base64"
    "io"
	"io/ioutil"
    "log"    
    "net/http"
	"sync"
	"errors"
)

var EmbedErrInvalid    = errors.New("invalid argument") // methods on embedFile will return this error when the receiver is nil
var EmbedErrPermission = errors.New("permission denied")
var EmbedErrExist      = errors.New("file already exists")
var EmbedErrNotExist   = errors.New("file does not exist")
var EmbedErrNotAFile   = errors.New("path is not a file")

// EmbedSetLocal tells embed to switch to local filesystem lookup by specifying 
// a filesystem path to act as your embed root. To set back to embeded mode, simply
// pass in a empty string
// example: mypkg.EmbedSetLocal("/path/to/folder") //local mode enabled
// example: mypkg.EmbedSetLocal("") //local mode disabled
var EmbedSetLocal = func(path string) {
	if path == "" {
		embedLocalMode = false
		embedLocalDir = ""
		return
	}
	
	embedLocalMode = true
	embedLocalDir = path
}

// EmbedHttpFS is the package-level generated http.FileSystem consumers can use 
// to serve the embeded content generated in this file.
var EmbedHttpFS = &embedFileSystem{}

// EmbedReadFile reads the file named by filename and returns the contents. A 
// successful call returns err == nil, not err == EOF. Because EmbedReadFile reads 
// the whole file, it does not treat an EOF from Read as an error to be reported. 
var EmbedReadFile = func(path string) ([]byte, error) {
	if embedLocalMode {
		return ioutil.ReadFile(path)
	}

	fi, ok := embedFilemap[path]

	if !ok {
		return []byte(""), EmbedErrNotExist
	}

	if fi.isDir {
		return []byte(""), EmbedErrNotAFile
	}

	if fi.decoded {
        return fi.data, nil
    }

	var data bytes.Buffer		

	b64 := base64.NewDecoder(base64.StdEncoding, bytes.NewBuffer(fi.data))
	gz, err := gzip.NewReader(b64)

	if err != nil {
		return []byte(""), err
	}

	if _, err = io.Copy(&data, gz); err != nil {
		return []byte(""), err
	}

	return data.Bytes(), nil	
}

// EmbedWriteFile writes data to a file named by filename. If the file does not 
// exist, EmbedWriteFile creates it with permissions perm; otherwise EmbedWriteFile 
// truncates it before writing.
var EmbedWriteFile = func(filename string, data []byte) error {	
	if embedLocalMode {
		return ioutil.WriteFile(filename, data, os.FileMode(0744))
	}

	fi, ok := embedFilemap[filename]

	if !ok {
		return EmbedErrNotExist
	}

	if fi.isDir {		
		return EmbedErrNotAFile
	}

	fi.mu.Lock()
	defer fi.mu.Unlock()

	if fi.decoded {
		fi.data = data
		return nil
	}

	var buf bytes.Buffer
	
	b64 := base64.NewEncoder(base64.StdEncoding, &buf)
	gz := gzip.NewWriter(b64)

	if _, err := io.Copy(gz, bytes.NewBuffer(data)); err != nil {
		return err
	}

	gz.Close()
	b64.Close()

	fi.data = buf.Bytes()
	fi.decoded = false

	return nil
}

// embedLocalMode indicates if this generated go file operates on the existing
// local filesystem or uses the embedded filesystem
var embedLocalMode = false

// embedLocalDir holds the real filesystem path to use when in local mode.
var embedLocalDir = ""

// embedFile impliments http.File interface
// see: https://golang.org/pkg/net/http/#File
type embedFile struct {
    reader   *bytes.Reader    
    info     *embedFileInfo
}

// Close ...
func (t *embedFile) Close() error {    
    return nil
}

// Read ...
func (t *embedFile) Read(p []byte) (int, error) {
    return t.reader.Read(p)
}

// Seek ...
// TODO: see if this is possible to impliment cleanly
func (t *embedFile) Seek(offset int64, whence int) (int64, error) {
    return t.reader.Seek(offset, whence)
}

// Readdir ...
// TODO: see if this is possible to impliment cleanly
func (t *embedFile) Readdir(count int) ([]os.FileInfo, error) {
    panic("Not Implimented")
    return nil, nil
}

// Stat ...
func (t *embedFile) Stat() (os.FileInfo, error) {
    return t.info, nil
}

// embedFileInfo impliments os.FileInfo
// see: https://golang.org/pkg/os/#FileInfo
type embedFileInfo struct {	
    path    string
    data    []byte
    decoded bool
	mu      *sync.Mutex

    name     string
    size     int64
    mode     uint32
	isDir    bool
}

// Name ...
func (t *embedFileInfo) Name() string {
    return t.name
}

// Size ...
func (t *embedFileInfo) Size() int64 {
    return t.size
}

// Mode ...
func (t *embedFileInfo) Mode() os.FileMode {    
    return os.FileMode(t.mode)
}

// ModTime ...
// TODO: pickup and provide actual mod time of the file during generation
func (t *embedFileInfo) ModTime() time.Time {
    return time.Now()
}

// IsDir ...
func (t *embedFileInfo) IsDir() bool {
    return t.isDir
}

// Sys ...
func (t *embedFileInfo) Sys() interface{} {    
    return nil
}

// embedFileSystem impliments http.FileSystem interface
// see: https://golang.org/pkg/net/http/#FileSystem
type embedFileSystem struct {
}

// Open ...
func (t *embedFileSystem) Open(name string) (http.File, error) {	
	if embedLocalMode {
		return os.Open(fmt.Sprintf("%v/%v", embedLocalDir, name))
	}

    f, ok := embedFilemap[name]

    if !ok {
        return nil, os.ErrNotExist
    }            

    if !f.decoded && !f.isDir { 
        var data bytes.Buffer		

        b64 := base64.NewDecoder(base64.StdEncoding, bytes.NewBuffer(f.data))
        gz, err := gzip.NewReader(b64)

        if err != nil {
            log.Println(err)
            return nil, err
        }

        if _, err := io.Copy(&data, gz); err != nil {
            log.Println(err)
            return nil, err
        }
		
        f.data = data.Bytes()
        f.decoded = true        
    }    

    file := &embedFile{
        info: f,
        reader: bytes.NewReader(f.data),
    }

    return file, nil		
}

// filemap holds the actual embeded file data
var embedFilemap = map[string]*embedFileInfo{
`
