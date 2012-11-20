package main

import (
	"errors"
	"fmt"
	"github.com/russross/blackfriday"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
)

var htmlFiles = []string{
	"html/view.html",
	"html/header.html",
	"html/footer.html",
	"html/dir.html",
}

var dataDir string

var templates *template.Template

func init() {
	templates = template.Must(template.ParseFiles(htmlFiles...))
}

type Page struct {
	Title string
	Body  []byte
	Url   string
}

type Link struct {
	Title string
	Path  string
}

// Create a list of links for the breadcrumb
func breadCrumb(path string) []Link {
	parts := strings.Split(path, "/")
	var crumbs []Link
	crumbs = append(crumbs, Link{Title: "Home", Path: "/"})
	subpath := "/"

	if len(parts) == 1 {
		return crumbs
	}

	for i := 1; i < len(parts); i++ {
		p := parts[i]
		if len(p) == 0 {
			break
		}
		title := strings.ToUpper(p[:1]) + p[1:]
		title = strings.Replace(title, "_", " ", 0)
		crumbs = append(crumbs, Link{Title: title, Path: subpath + p})
		subpath = subpath + p + "/"
	}

	return crumbs
}

func loadDir(path string) ([]Link, error) {
	path = path[1:]
	if len(path) == 0 || path[:1] == "/" {
		return nil, errors.New("Path not found")
	}

	dirname := dataDir + path
	files, err := ioutil.ReadDir(dirname)
	if err != nil {
		return nil, err
	}

	var links []Link
	for _, file := range files {
		f := file.Name()
		if f[0] == '.' {
			continue
		}
		if len(f) > 3 && f[len(f)-3:] == ".md" {
			f = f[:len(f)-3]
		}
		links = append(links, Link{f, "/" + path + "/" + f})
	}
	return links, nil
}

func loadPage(path string) (*Page, error) {
	path = path[1:]
	if len(path) == 0 {
		path = path + "index"
	} else if path[len(path)-1:] == "/" {
        // strip off / in case there's a .md one dir up
        path = path[:len(path)-1]
    } else if len(path) > 3 && path[len(path)-3:] == ".md" {
        path = path[:len(path)-3]
    }
	filename := dataDir + path + ".md"
	body, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, errors.New("Page not found: " + path)
	}
	url := "/" + path
	return &Page{path, body, url}, nil
}

func dirHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	dir, err := loadDir(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		log.Println(err)
		return
	}
	renderTemplate(w, r, "header", breadCrumb(r.URL.Path))
	renderTemplate(w, r, "dir", dir)
	renderTemplate(w, r, "footer", nil)
}

func fileHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[1:]
	filename := dataDir + path
	file, err := ioutil.ReadFile(filename)
	if err != nil {
		dirHandler(w, r)
		return
	}
	fmt.Fprintf(w, "%s", file)
}

func pageHandler(w http.ResponseWriter, r *http.Request) {
	// verify the file path
	path := r.URL.Path
	// load up the file
	page, err := loadPage(path)
	if err != nil {
		page, err = loadPage(path + "/index")
		if err != nil {
			fileHandler(w, r)
			return
		}
	}
	html := template.HTML(blackfriday.MarkdownCommon(page.Body))
	out := struct {
		Title string
		Body  []byte
		Html  template.HTML
	}{
		page.Title,
		page.Body,
		html,
	}
	// pass the file into the view template
	renderTemplate(w, r, "header", breadCrumb(r.URL.Path))
	renderTemplate(w, r, "view", out)
	renderTemplate(w, r, "footer", nil)
}

func renderTemplate(w http.ResponseWriter, r *http.Request, tmpl string, data interface{}) {
	err := templates.ExecuteTemplate(w, tmpl+".html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Must have a data directory argument.")
	}
	dataDir = os.Args[1]
	if dataDir[len(dataDir)-1] != '/' {
		dataDir = dataDir + "/"
	}
	http.HandleFunc("/", pageHandler)
	http.ListenAndServe("127.0.0.1:6969", nil)
}
