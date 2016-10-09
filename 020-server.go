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

var logchan = make(chan *http.Request, *maxusers) // HandleFuncs can send req to this chan to log it.
var totalhits int                                 // Main hit counter
var lim = map[string]int{}                        // Limit number resets every second. Can't be more than 20 per 10 seconds.
var visitor = map[string]*Limiting{}              // Visitor number is the totalhits when visitor was unique.
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
// If this returns true, the parent function should continue
func ifCachedDo(w http.ResponseWriter, r *http.Request) bool {
	logchan <- r // logchan limits global users and populates the visitor map

	ip := getip(r.RemoteAddr)

	// every HandlerFunc must empty the ratelimiter when finished (defer unlimit())
	// logchan and ratelimit together will limit the amount of traffic to the server.
	ratelimit <- Hit{Time: time.Now(), IP: ip}

	var limited bool
	if *debug {
		log.Println("Locking for cache")
	}
	mutex.Lock()
	if visitor[ip] != nil {
		limited = visitor[ip].RateLimited
	} else {
		visitor[ip] = &Limiting{Since: time.Now()}
	}
	mutex.Unlock()
	if *debug {
		log.Println("UnLocking for cache")
	}

	if limited {
		log.Println("Not serving, rate limited:", ip)
		http.Redirect(w, r, "/?limit", http.StatusForbidden)
		unlimit()
		return false
	}

	// only cache GETs
	if r.Method != "GET" {
		return true
	}
	path := r.RequestURI
	cached, err := c1.Get(path)
	if err != nil {
		// Returns an error if not found, lets create it.
		if err.Error() == "not found" {
			log.Printf("Creating cache for %q.", path)
			c1.Set(path, []byte(""))
			return true
		}

		// Real error
		log.Println("Cache error:", err)
		http.Redirect(w, r, "/", http.StatusInternalServerError)
		unlimit()
		return false
	}

	// Has a cache.
	if cached != nil {
		log.Println("Requested thumbnail is cached. Not resizing.")
		b := cached.([]byte)
		w.Write(b)
		unlimit() // Empty ratelimiter 1
		return false
	}

	return true
}

// Empty the ratelimiter by one
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
		if *debug {
			log.Println("Cache parse took:", t2.Sub(t1))
		}
		return
	}
	t2 = time.Now()
	if *debug {
		log.Println("NotUsingCache parse took:", t2.Sub(t1))
	}
	t1 = time.Now()

	defer unlimit()

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

	// Check ID. (Could be 'filename.png')
	id := vars["id"]
	log.Println("Checking ID", id)
	dotsplit := strings.Split(id, ".")
	id = dotsplit[0] // 'filename' not 'png'
	if len(dotsplit) == 1 {
		log.Println("Going to resize!", id)
		goto Resize
	}
	if len(dotsplit) > 2 { // Funny extension that we dont support yet
		log.Println("Going to error!", id)
		dotsplit[1] = "error"
	}

	// dotsplit[1] of 'filename.png' would be 'png'
	switch dotsplit[1] {
	case "jpeg": // 4
	case "jpg", "png", "gif": //3
	default: // everything else including "error" extension
		log.Println("Bad extension.", dotsplit)
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	id = dotsplit[0]

Resize:
	if len(id) != *filenameLength {
		http.Redirect(w, r, "/", http.StatusFound)
		log.Println(id, len(id), "!=", *filenameLength)
		return
	}
	if *debug {
		log.Println("Request parse took:", t2.Sub(t1))
	}
	// Seems legit

	if *debug {
		log.Printf("Resize Request %q to: %vx%v", id, width, height)
	}
	t1 = time.Now()
	image := getimage(id)
	if image == nil {
		if *debug {
			log.Println("Nil image")
		}
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	t2 = time.Now()
	if *debug {
		log.Println("Image read took:", t2.Sub(t1))
	}
	t1 = time.Now()
	newImage := imaging.Resize(image, width, height, imaging.Lanczos)
	t2 = time.Now()
	if *debug {
		log.Println("Image resize took:", t2.Sub(t1))
	}
	t1 = time.Now()
	// Encode the bytes, using a buffer so we can cache it.
	// Otherwise this would be much more simple of a function. (jpeg.Encode(w, newImage,nil))
	var buf bytes.Buffer
	defer buf.Reset()
	jpeg.Encode(&buf, newImage, nil)
	t2 = time.Now()
	if *debug {
		log.Println("Image encoding took:", t2.Sub(t1))
	}
	e := c1.Set(r.RequestURI, buf.Bytes())
	if e != nil {
		log.Println(e)
		w.Write([]byte("Image Encoding Error"))
		return
	}
	w.Write(buf.Bytes())
	if *debug {
		log.Printf("Created Cache: %vx%v %s (%v bytes in memory)", width, height, id, buf.Len())
	}
}

// Home page HTML form, caching disabled so we can redirect limited to home
func s0Home(w http.ResponseWriter, r *http.Request) {
	// if !ifCachedDo(w, r) {
	// 	return
	// }
	// defer unlimit()
	// e := c1.Set(r.RequestURI, []byte(header+form+footer))
	// if e != nil {
	// 	log.Println(e)
	// }
	logchan <- r
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

	ip := getip(r.RemoteAddr)
	if visitor[ip] != nil {
		if visitor[ip].RateLimited {
			log.Println("Not serving, rate limited:", ip)
			http.Redirect(w, r, "/?limit", http.StatusForbidden)
			return
		}
	}

	if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "multipart/form-data" {
		log.Println("Not uploading, bad form.", ip, r.Header.Get("Content-Type"))
		http.Redirect(w, r, "/?bad", http.StatusForbidden)
		return
	}

	_, fileheader, err := r.FormFile("file")
	if err != nil {
		log.Println(err)
		http.Redirect(w, r, "/?bad", http.StatusForbidden)
		return
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
