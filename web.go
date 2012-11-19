package main

import (
	"errors"
	"fmt"
	"github.com/russross/blackfriday"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strings"
)

var htmlFiles = []string{
	"html/view.html",
	"html/header.html",
	"html/footer.html",
	"html/dir.html",
}

var dataDir = "chrissexton.org/site/"
var staticDir = "chrissexton.org/"
var siteUrl = "http://127.0.0.1:8080"

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

	path = dataDir + path
	log.Println("Loading directory: ", path)
	files, err := ioutil.ReadDir(path)
	if err != nil {
		log.Println(err)
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
		links = append(links, Link{f, path + "/" + f})
	}
	return links, nil
}

func loadPage(path string) (*Page, error) {
	path = path[1:]
	if len(path) == 0 || path[:1] == "/" {
		path = path + "index"
	}
	filename := dataDir + path + ".md"
	log.Println("Loading ", filename)
	body, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Println(err)
		return nil, errors.New("Page not found: " + path)
	}
	url := siteUrl + "/" + path
	return &Page{path, body, url}, nil
}

const lenPath = len("/view/")

func viewHandler(w http.ResponseWriter, r *http.Request, title string) {
	p, err := loadPage(title)
	if err != nil {
		http.Redirect(w, r, "/edit/"+title, http.StatusFound)
		return
	}
	renderTemplate(w, r, "view", p)
}

func pageHandler(w http.ResponseWriter, r *http.Request) {
	// verify the file path
	path := r.URL.Path
	// load up the file
	page, err := loadPage(path)
	if err != nil {
		page, err = loadPage(path + "/index")
	}
	if err != nil {
		dir, err := loadDir(path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		renderTemplate(w, r, "header", breadCrumb(r.URL.Path))
		renderTemplate(w, r, "dir", dir)
		renderTemplate(w, r, "footer", nil)
		return
	}
	html := template.HTML(blackfriday.MarkdownCommon(page.Body))
	// out := struct{Title: p.Title, Body: p.Body, Html: html}
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

func staticHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[1:]
	if len(path) == 0 || path[:1] == "/" {
		http.Error(w, "Not found", http.StatusNotFound)
	}
	filename := staticDir + path
	log.Println("Loading ", filename)
	file, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Println(err)
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	fmt.Fprintf(w, "%s", file)
}

var titleValidator = regexp.MustCompile("([A-Z][a-z0-9])+")

func makeHandler(fn func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		title := r.URL.Path[lenPath:]
		if !titleValidator.MatchString(title) {
			http.NotFound(w, r)
			return
		}
		fn(w, r, title)
	}
}

func main() {
	http.HandleFunc("/favicon.ico", staticHandler)
	http.HandleFunc("/pub/", staticHandler)
	http.HandleFunc("/", pageHandler)
	http.ListenAndServe(":8080", nil)
}
