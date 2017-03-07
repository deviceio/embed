# Embed

Generates golang source files with resources embedded into them. Ship your assets with your binary!

# Install

```golang
go get github.com/deviceio/embed
go install github.com/deviceio/embed
```

# Usage

Pick a path somewhere in your project to store embeded assets. This path must be a folder. After running the `embed` cli tool from $GOPATH/bin a new golang source file will be present in your choosen path containing embedded file data for all files found in that path and its child folders.

For example purposes we are going to assume we are embedding our `www` folder in our project to ship our browser code with the binary

```golang
cd $GOPATH/src
mkdir myproject/www
echo "<h1>Hello, World</h1>" > myproject/www/index.html
embed myproject/www/data.go
```

Review the newly generated `data.go` file in your www path. You are welcome to change the name of the file in the cli command or manually afterwords. The newly generated source file's `package` has inherited the folder name it was placed in. You can now reference functionality within your embeded file elsewhere in your application:

For example lets host a http file server directlyfrom our embedded assets

```golang
import(
    "myproject/www"
)

func main() {
    http.ListenAndServe(":8081", http.FileServer(www.EmbedHttpFS))
}
```

```golang
go run myproject/main.go
```

browse `http://127.0.0.1:8081/` to see your embedded binary assets served over http!

