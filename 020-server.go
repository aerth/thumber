package main

import (
	"bytes"
	"image"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aerth/filer"
	"github.com/disintegration/imaging"
	"github.com/gorilla/mux"
)

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
	im := getimage(id)
	if im == nil {
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

	// Resize the image
	resizechan := make(chan image.Image, 1)
	go func() {
		t1 = time.Now()
		resized := imaging.Resize(im, width, height, imaging.Lanczos)

		t2 = time.Now()
		if *debug {
			log.Println("Image resize took:", t2.Sub(t1))
		}
		resizechan <- resized
	}()
	var resized image.Image
	select {
	case <-time.After(5 * time.Second):
		log.Println("Timeout resizing.")
		http.Redirect(w, r, "/?timeout", http.StatusBadRequest)
		return
	case incoming := <-resizechan:
		resized = incoming
	}
	// Encode the bytes, using a buffer so we can cache it.
	// Otherwise this would be much more simple of a function. (jpeg.Encode(w, newImage,nil))
	t1 = time.Now()
	var buf bytes.Buffer
	defer buf.Reset()
	imagechan := DetectFormat(buf, resized)
	// switch on first receive
	var ithing Imagething
	select {
	case thing := <-imagechan:
		ithing = thing
		switch thing.ext {
		case "png":
			log.Println("Got a PNG")
		case "jpg":
			log.Println("Got a JPG")
		case "gif":
			log.Println("Got a GIF")
		default:
			log.Println("What the fuck")
		}
	case <-time.After(5 * time.Second):
		log.Println("Timeout. Cancelling request.")
		http.Redirect(w, r, "/?timeout", http.StatusBadRequest)
		return
	}

	t2 = time.Now()
	if *debug {
		log.Println("Image encoding took:", t2.Sub(t1))
	}
	e := c1.Set(r.RequestURI, ithing.buf.Bytes())
	if e != nil {
		log.Println(e)
		w.Write([]byte("Image Encoding Error"))
		return
	}
	w.Write(ithing.buf.Bytes())
	if *debug {
		log.Printf("Created Cache: %vx%v %s (%v bytes in memory)", width, height, id, ithing.buf.Len())
	}
}

// Home page HTML form, caching disabled so we can redirect limited to home
func s0Home(w http.ResponseWriter, r *http.Request) {
	logchan <- r
	w.Write([]byte(header + form + footer))
}

// Return an original size image (cached+ratelimited)
func s0Get(w http.ResponseWriter, r *http.Request) {
	if !ifCachedDo(w, r) {
		return
	}
	defer unlimit()

	// id from URL
	vars := mux.Vars(r)
	id := vars["id"]
	if id == "" {
		http.Redirect(w, r, "/", http.StatusFound)
	}

	// Get image bytes directly from file.
	b := getbytes(id)

	// Set cache for URL
	e := c1.Set(r.RequestURI, b)
	if e != nil {
		log.Println(e)
	}

	// Write to http response
	w.Write(b)
}

// Upload an image (POST) and forward to a resized version.
// The "uploaded" image is written exactly as the server receives it.
// We don't know whether its a PNG, JPEG, or EXE at this point.
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

	log.Println("DEBUG", fileheader.Header)
	nameparts := strings.Split(fileheader.Filename, ".")
	extension := nameparts[len(nameparts)-1]
	log.Println("Uploading:", fileheader.Filename, extension)

	// Read file into memory
	openfile, err := fileheader.Open()
	if err != nil {
		log.Println(r, err)
		return
	}

	// Read the file into buffer (only to cache)
	var buf bytes.Buffer
	i, e := buf.ReadFrom(openfile)
	if e != nil {
		log.Println(r, i, e)
		return
	}

	// Generate new ID
	id := unique()

	// Write the file.
	filer.Touch(*uploadsDir + id)
	filer.Write(*uploadsDir+id, buf.Bytes())
	log.Println("Uploaded:", *uploadsDir+id)

	// Redirect to a 120xAutoHeight thumbnail
	http.Redirect(w, r, "/120/0/"+id, http.StatusFound)
}
