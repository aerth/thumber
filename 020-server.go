package main

import (
	"bytes"
	"image/jpeg"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aerth/filer"
	"github.com/disintegration/imaging"
	"github.com/drone/drone/cache"
	"github.com/gorilla/mux"
)

var logchan = make(chan *http.Request, 1) // HandleFuncs can send req to this chan to log it.
var totalhits int                         // Main hit counter
var lim = map[string]int{}                // Limit number resets every second. Can't be more than 20 per 10 seconds.
var visitor = map[string]int{}            // Visitor number is the totalhits when visitor was unique.
var ratelimit = make(chan Hit, *maxusers)

// Hit is a visitor, in rate-limiting context. Currently unimplemented.
type Hit struct {
	IP   string
	Time time.Time
	Path string
}

var c1 cache.Cache

func ratelimiter() {}

// Quick! Log the request while limiting hit rate. Return false if cached.
func ifCachedDo(w http.ResponseWriter, r *http.Request) bool {
	logchan <- r // logchan limits global users
	var hit Hit
	hit.IP = getip(r.RemoteAddr)
	hit.Time = time.Now()
	hit.Path = r.RequestURI
	ratelimit <- hit       // every func must empty the ratelimiter when finished (defer unlimit())
	if r.Method != "GET" { // only cache GETs
		return true
	}
	cached, err := c1.Get(hit.Path)
	if err != nil {
		if err.Error() == "not found" {
			log.Printf("Creating cache for %q.", hit.Path)
			c1.Set(hit.Path, []byte(""))
			return true
		}
		log.Println("Cache error:", err)
		http.Redirect(w, r, "/", http.StatusFound)
		unlimit() // Empty ratelimiter 1
		return false
	}
	if cached != nil {
		log.Println("Requested thumbnail is cached. Not resizing.")
		b := cached.([]byte)
		w.Write(b)
		unlimit() // Empty ratelimiter 1
		return false
	}
	log.Println("Not cached yet, let's resize!")
	return true
}

func unlimit() {
	<-ratelimit
}

// Split something like 10.4.2.0:32040 into 10.4.2.0
func getip(req string) string {
	return strings.Split(req, ":")[0]
}

var t1, t2 time.Time

// Return a resized image based on request path variables
func s0Resize(w http.ResponseWriter, r *http.Request) {
	t1 = time.Now()
	if !ifCachedDo(w, r) {
		t2 = time.Now()
		log.Println("Cache parse took:", t2.Sub(t1))
		return
	}
	t2 = time.Now()
	log.Println("Cache parse took:", t2.Sub(t1))
	t1 = time.Now()

	defer unlimit()
	log.Println()
	// Decode URL, check width and height
	vars := mux.Vars(r)
	width, err := strconv.Atoi(vars["w"])
	if err != nil {
		log.Println(err)
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	height, err := strconv.Atoi(vars["h"])
	if err != nil {
		log.Println(err)
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if height > 10000 || width > 10000 {
		log.Println("Too Large.")
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if height < 3 && width < 3 {
		log.Println("Too Small.")
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	// Check ID
	id := vars["id"]
	if len(id) != 10 {
		http.Redirect(w, r, "/", http.StatusFound)
		log.Println(len(id), "!= 10")
		return
	}
	log.Println("Request parse took:", t2.Sub(t1))
	// Seems legit
	log.Printf("Resize Request %q to: %vx%v", id, width, height)
	t1 = time.Now()
	image := getimage(id)
	if image == nil {
		log.Println("Nil image")
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	t2 = time.Now()
	log.Println("Image read took:", t2.Sub(t1))
	t1 = time.Now()
	newImage := imaging.Resize(image, width, height, imaging.Lanczos)
	t2 = time.Now()
	log.Println("Image resize took:", t2.Sub(t1))
	t1 = time.Now()
	// Encode the bytes, using a buffer so we can cache it.
	var buf bytes.Buffer
	defer buf.Reset()
	jpeg.Encode(&buf, newImage, nil)
	t2 = time.Now()
	log.Println("Image encoding took:", t2.Sub(t1))
	e := c1.Set(r.RequestURI, buf.Bytes())
	if e != nil {
		log.Println(e)
		w.Write([]byte("Image Encoding Error"))
		return
	}
	w.Write(buf.Bytes())
	log.Printf("Cached: %vx%v %s (%v bytes in memory)", width, height, id, buf.Len())
}

// Home page HTML form
func s0Home(w http.ResponseWriter, r *http.Request) {
	if !ifCachedDo(w, r) {
		return
	}
	defer unlimit()
	e := c1.Set(r.RequestURI, []byte(header+form+footer))
	if e != nil {
		log.Println(e)
	}
	w.Write([]byte(header + form + footer))
}

// Return an original size image
func s0Get(w http.ResponseWriter, r *http.Request) {
	if !ifCachedDo(w, r) {
		return
	}
	defer unlimit()
	vars := mux.Vars(r)
	id := vars["id"]
	if id == "" {
		http.Redirect(w, r, "/", http.StatusFound)
	}
	e := c1.Set(r.RequestURI, getbytes(id))
	if e != nil {
		log.Println(e)
	}
	w.Write(getbytes(id))

}

// Upload an image (POST) and forward to a resized version.
func s0Upload(w http.ResponseWriter, r *http.Request) {
	if !ifCachedDo(w, r) {
		return
	}
	defer unlimit()
	_, fileheader, err := r.FormFile("file")
	if err != nil {
		panic(err)
	}
	nameparts := strings.Split(fileheader.Filename, ".")
	extension := nameparts[len(nameparts)-1]
	log.Println("Uploading:", fileheader.Filename, extension)
	openfile, err := fileheader.Open()
	if err != nil {
		log.Println(r, err)
		return
	}
	var buf bytes.Buffer
	i, e := buf.ReadFrom(openfile)
	if e != nil {
		log.Println(r, i, e)
		return
	}
	id := unique()
	filer.Touch(*uploadsDir + id)
	filer.Write(*uploadsDir+id, buf.Bytes())
	log.Println("Uploaded:", *uploadsDir+id)
	http.Redirect(w, r, "/300/0/"+id, http.StatusFound)
}
