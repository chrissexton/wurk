package main

import (
	"errors"
	"fmt"
	"github.com/russross/blackfriday"
	"go/build"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	importString = "bitbucket.org/phlyingpenguin/website"
)

// Cache for template files
var templates map[string]*template.Template

type Link struct {
	Title string
	Path  string
}

// Create a slice of Link for the breadcrumb
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
		title = strings.Replace(title, "_", " ", -1)
		crumbs = append(crumbs, Link{Title: title, Path: subpath + p})
		subpath = subpath + p + "/"
	}

	return crumbs
}

// Produce a []Link to provide directory listings
func loadDir(r *http.Request, path string) ([]Link, error) {
	if len(path) == 0 || path[:1] == "/" {
		return nil, errors.New("Path not found")
	}

	files, err := ioutil.ReadDir(path)
	if err != nil {
		log.Println("Couldn't load path ", path)
		return nil, err
	}

	cache := make(map[string]bool)
	var links []Link
	for _, file := range files {
		f := file.Name()
		// No hidden files to allow disabling files
		if f[0] == '.' {
			continue
		}
		if len(f) > 3 && f[len(f)-3:] == ".md" {
			f = f[:len(f)-3]
		}
		if _, ok := cache[f]; !ok {
			links = append(links, Link{f, getUrl(r, path) + f})
			cache[f] = true
		}
	}
	return links, nil
}

// Open the actual markdown files for service
// This attempts to open any file it possibly can to prevent
// later loaders from taking over
func loadPage(path string) ([]byte, error) {
	if len(path) == 0 {
		path = filepath.Join(path, "index")
	} else if path[len(path)-1:] == "/" {
		// strip off / in case there's a .md one dir up
		path = path[:len(path)-1]
	} else if len(path) > 3 && path[len(path)-3:] == ".md" {
		path = path[:len(path)-3]
	}
	filename := path + ".md"
	body, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, errors.New("Page not found: " + path)
	}
	return body, nil
}

// Serve an index of any directory that hasn't been hit yet
// Note: put an index.md in any directory that should not be
// globally accessible.
func dirHandler(w http.ResponseWriter, r *http.Request) {
	path := getPubPath(r)
	dir, err := loadDir(r, path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		log.Println(err)
		return
	}
	renderTemplate(w, r, "header", breadCrumb(r.URL.Path))
	renderTemplate(w, r, "dir", dir)
	renderTemplate(w, r, "footer", nil)
}

// Serve any raw files that may be in the directory
// Note: this does not pass proper MIME types
// This passes through to the dirHandler
func fileHandler(w http.ResponseWriter, r *http.Request) {
	path := getPubPath(r)
	filename := path
	file, err := ioutil.ReadFile(filename)
	if err != nil {
		dirHandler(w, r)
		return
	}
	fmt.Fprintf(w, "%s", file)
}

// Main handler funnction, tries to load any .md pages
// This passes through to the fileHandler (and then to dirHandler)
func pageHandler(w http.ResponseWriter, r *http.Request) {
	if err := checkDomain(w, r); err != nil {
		return
	}
	path := getPubPath(r)
	page, err := loadPage(path)
	if err != nil {
		page, err = loadPage(filepath.Join(path, "index"))
		if err != nil {
			fileHandler(w, r)
			return
		}
	}
	html := template.HTML(blackfriday.MarkdownCommon(page))
	// pass the file into the view template
	renderTemplate(w, r, "header", breadCrumb(r.URL.Path))
	renderTemplate(w, r, "view", html)
	renderTemplate(w, r, "footer", nil)
}

// Try to load and execute a template for the given site
func renderTemplate(w http.ResponseWriter, r *http.Request, tmpl string, data interface{}) {
	tPath := filepath.Join(getTmplPath(r), tmpl+"html")
	t, ok := templates[tPath]
	var err error
	if !ok {
		t, err = template.ParseFiles(filepath.Join(getTmplPath(r), tmpl+".html"))
		if err != nil {
			http.Error(w, "Could not load templates.", http.StatusInternalServerError)
			log.Println(err)
			return
		}
		templates[tPath] = t
	}
	err = t.Execute(w, data)
	if err != nil {
		http.Error(w, "Could not load templates.", http.StatusInternalServerError)
		log.Println(err)
	}
}

// Check for requisite domain files, if none exist, redirect to an error page
func checkDomain(w http.ResponseWriter, r *http.Request) error {
	if _, err := os.Stat(filepath.Join(r.Host, "pub")); err != nil {
		goto errpage
	}
	if _, err := os.Stat(filepath.Join(r.Host, "templates")); err != nil {
		goto errpage
	}
	return nil
errpage:
	pkg, err := build.Import(importString, "", build.FindOnly)
	if err != nil {
		http.Error(w, "Error page unrenderable", http.StatusInternalServerError)
		return errors.New("Terrible failure!")
	}
	p := filepath.Join(pkg.Dir, "domainerror.html")
	t, err := template.ParseFiles(p)
	if err != nil {
		http.Error(w, "Error page unrenderable", http.StatusInternalServerError)
		return errors.New("Terrible failure!")
	}
	t.Execute(w, r.Host)
	return errors.New("Domain not found.")
}

// Extract url from local file path
func getUrl(r *http.Request, path string) string {
	return strings.Replace(path, r.Host+"/pub", "", 1) + "/"
}

// Take URL path and return local public path (based on hostname)
func getPubPath(r *http.Request) string {
	return filepath.Join(r.Host, "/pub", r.URL.Path)
}

// Take URL path and return local template path (based on hostname)
func getTmplPath(r *http.Request) string {
	return filepath.Join(r.Host, "/templates/")
}

func main() {
	http.HandleFunc("/", pageHandler)
	log.Fatal(http.ListenAndServe("0.0.0.0:6969", nil))
}

func init() {
	templates = make(map[string]*template.Template)
}
