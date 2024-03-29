package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"github.com/gernest/front"
	"github.com/russross/blackfriday/v2"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	domainError = `Sorry, this server doesn't know how to serve {{.}}!`
)

// PageInfo tracks any information given to templates
type PageInfo struct {
	BreadCrumb []Link
	Title      string
	RawDate    time.Time
	Date       string
	Time       string
	Author     string
	Dir        []Link
	Page       template.HTML
}

// Cache for template files
type templateCache struct {
	t  *template.Template
	ts time.Time
}

var templates map[string]templateCache

type Link struct {
	Title string
	Path  string
}

// Create a slice of Link for the breadcrumb
func breadCrumb(path string) []Link {
	parts := strings.Split(path, "/")
	var crumbs []Link
	crumbs = append(crumbs, Link{Title: "Home", Path: "/"})
	subPath := "/"

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
		crumbs = append(crumbs, Link{Title: title, Path: subPath + p})
		subPath = subPath + p + "/"
	}

	return crumbs
}

// Produce a []Link to provide directory listings
func loadDir(r *http.Request, path string) ([]Link, error) {
	if len(path) == 0 || path[:1] == "/" {
		return nil, errors.New("Path not found")
	}

	files, err := os.ReadDir(path)
	if err != nil {
		log.Println("Couldn't load path ", path)
		return nil, err
	}

	cache := make(map[string]bool)
	var links []Link
	for _, file := range files {
		f := file.Name()
		// No hidden files to allow disabling files
		if f[0] == '.' || f == "_index.md" {
			continue
		}
		if len(f) > 3 && f[len(f)-3:] == ".md" {
			f = f[:len(f)-3]
		}
		if _, ok := cache[f]; !ok {
			trailing := ""
			if file.IsDir() {
				trailing = "/"
			}
			links = append(links, Link{f, getUrl(r, path) + f + trailing})
			cache[f] = true
		}
	}
	return links, nil
}

// Open the actual markdown files for service
// This attempts to open any file it possibly can to prevent
// later loaders from taking over
func loadPage(path string) (template.HTML, map[string]interface{}, error) {
	if len(path) == 0 {
		path = filepath.Join(path, "index")
	} else if path[len(path)-1:] == "/" {
		// strip off / in case there's a .md one dir up
		path = path[:len(path)-1]
	} else if len(path) > 3 && path[len(path)-3:] == ".md" {
		path = path[:len(path)-3]
	}
	filename := path + ".md"
	fileContents, err := os.ReadFile(filename)
	if err != nil {
		return "", nil, errors.New("Page not found: " + path)
	}
	m := front.NewMatter()
	m.Handle("---", front.YAMLHandler)
	f, body, err := m.Parse(bytes.NewBuffer(fileContents))
	html := template.HTML(blackfriday.Run([]byte(body)))
	return html, f, nil
}

// Try to load an index.html file, maybe fail
func htmlIndex(w http.ResponseWriter, r *http.Request) bool {
	path := getPubPath(r)
	filename := path + "/index.html"
	file, err := ioutil.ReadFile(filename)
	if err != nil {
		return false
	}
	fmt.Fprintf(w, "%s", file)
	return true
}

// Serve an index of any directory that hasn't been hit yet
// Note: put an index.md in any directory that should not be
// globally accessible.
func dirHandler(w http.ResponseWriter, r *http.Request) {
	path := getPubPath(r)
	dir, err := loadDir(r, path)
	if err != nil {
		msg := fmt.Sprintf("Could not load %s: File not found", r.URL.Path)
		http.Error(w, msg, http.StatusNotFound)
		log.Println(err)
		return
	}

	if htmlIndex(w, r) {
		return
	}
	summary, f, err := loadPage(path + "/_index.md")
	info := NewPageInfo(f)
	info.BreadCrumb = breadCrumb(r.URL.Path)
	info.Dir = dir
	info.Page = summary
	renderTemplate(w, r, "header", info)
	if err == nil {
		renderTemplate(w, r, "view", info)
	}
	renderTemplate(w, r, "dir", info)
	renderTemplate(w, r, "footer", info)
}

// Serve any raw files that may be in the directory
// Note: this does not pass proper MIME types
// This passes through to the dirHandler
func fileHandler(w http.ResponseWriter, r *http.Request) {
	path := getPubPath(r)
	filename := path
	_, err := ioutil.ReadFile(filename)
	if err != nil {
		dirHandler(w, r)
		return
	}
	http.ServeFile(w, r, filename)
}

// Main handler funnction, tries to load any .md pages
// This passes through to the fileHandler (and then to dirHandler)
func pageHandler(w http.ResponseWriter, r *http.Request) {
	if err := checkDomain(w, r); err != nil {
		return
	}
	path := getPubPath(r)
	page, f, err := loadPage(path)
	if err != nil {
		page, f, err = loadPage(filepath.Join(path, "index"))
		if err != nil {
			fileHandler(w, r)
			return
		}
	}
	info := NewPageInfo(f)
	info.BreadCrumb = breadCrumb(r.URL.Path)
	info.Page = page
	// pass the file into the view template
	renderTemplate(w, r, "header", info)
	renderTemplate(w, r, "view", info)
	renderTemplate(w, r, "footer", info)
}

// Try to load and execute a template for the given site
func renderTemplate(w http.ResponseWriter, r *http.Request, tmpl string, data PageInfo) {
	tPath := filepath.Join(getTmplPath(r), tmpl+"html")
	tc, ok := templates[tPath]
	var err error
	if !ok || tc.ts.Before(time.Now().Add(-*cacheTimeout)) {
		tc.t, err = template.ParseFiles(filepath.Join(getTmplPath(r), tmpl+".html"))
		if err != nil {
			http.Error(w, "Could not load templates.", http.StatusInternalServerError)
			log.Println(err)
			return
		}
		templates[tPath] = templateCache{
			t:  tc.t,
			ts: time.Now(),
		}
	}
	err = tc.t.Execute(w, data)
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
	tmpl := template.New("domainError")
	t, err := tmpl.Parse(domainError)
	if err != nil {
		http.Error(w, "Error page unrenderable", http.StatusInternalServerError)
		return errors.New("terrible failure")
	}
	t.Execute(w, r.Host)
	return errors.New("domain not found")
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

var addr = flag.String("addr", "0.0.0.0:6969", "Where")
var cacheTimeout = flag.Duration("cacheTimeout", time.Minute, "cache timeout duration")

func main() {
	flag.Parse()
	http.HandleFunc("/", pageHandler)
	log.Println("Listening on http://" + *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func init() {
	templates = make(map[string]templateCache)
}

func NewPageInfo(f map[string]interface{}) PageInfo {
	t := time.Now()
	pi := PageInfo{
		RawDate: t,
		Date:    "",
		Time:    "",
	}
	if t, ok := f["time"]; ok {
		pi.Time = t.(string)
	}
	if d, ok := f["date"]; ok {
		pi.Date = d.(string)
	} else {
		pi.Date = t.Format(time.DateOnly)
		pi.Time = t.Format("15:04")
	}
	if a, ok := f["author"]; ok {
		pi.Author = a.(string)
	}
	if t, ok := f["title"]; ok {
		pi.Title = t.(string)
	}
	return pi
}
