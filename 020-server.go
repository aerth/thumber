package main

import (
	"bytes"
	"image/gif"
	"image/jpeg"
	"image/png"
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

func s0ResizeExt(w http.ResponseWriter, r *http.Request) {

	log.Println("New resizor")
	vars := mux.Vars(r)
	id := vars["id"]
	width, _ := strconv.Atoi(vars["w"])
	height, _ := strconv.Atoi(vars["h"])
	ext := vars["ext"]
	if id == "" || ext == "" {
		log.Println(id, ext, "blank one")
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if len(id) != *filenameLength {
		log.Println(id, len(id), "!=", *filenameLength)
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	log.Println("Getting image:", id)
	t1 = time.Now()
	im := getimage(id)
	if im == nil {

		log.Println("Nil image")

		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	t2 = time.Now()
	if *debug {
		log.Println("Image read took:", t2.Sub(t1))
	}
	resized := imaging.Resize(im, width, height, imaging.Lanczos)
	var b bytes.Buffer

	switch ext {
	case "png":
		er := png.Encode(&b, resized)
		if er != nil {
			log.Println(er)
		}
	case "jpeg":
		er := jpeg.Encode(&b, resized, nil)
		if er != nil {
			log.Println(er)
		}
	case "gif":
		er := gif.Encode(&b, resized, nil)
		if er != nil {
			log.Println(er)
		}
	default:
		log.Println(ext, "what?")
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	w.Write(b.Bytes())
}

// Home page HTML form, caching disabled so we can redirect limited to home
func s0Home(w http.ResponseWriter, r *http.Request) {

	logchan <- r
	if r.URL.Path != "/" || r.Method != "GET" {
		log.Println("Home Redirecting:", r.URL.Path)
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	w.Write([]byte(header + form + footer))
}

// Return an original size image (no encoding, cached and ratelimited)
func s0Get(w http.ResponseWriter, r *http.Request) {
	log.Println("s0Get")

	if !ifCachedDo(w, r) {
		return
	}
	defer unlimit()

	// id from URL
	vars := mux.Vars(r)
	id := vars["id"]
	if id == "" {
		log.Println("no id")
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	// Don't use extension, but test it.
	ext := vars["ext"]
	switch ext {
	case "png", "jpg", "jpeg", "gif":
	default:
		http.Redirect(w, r, "/"+id, http.StatusFound)
		return
	}
	_ = ext

	// Get image bytes directly from file.
	b, e := getbytes(id)
	if e != nil {
		if *debug {
			log.Println("Image not found,", e)
		}
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if b == nil {
		log.Println("Image is 0 bytes")
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	// Set cache for URL
	e = c1.Set(r.RequestURI, b)
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
	if !ifCachedDo(w, r) { // we dont cache here but we rate limit and log
		return
	}
	defer unlimit()
	ip := getip(r.RemoteAddr)
	if visitor[ip] != nil {
		if visitor[ip].RateLimited {
			log.Println("Not uploading, rate limited:", ip)
			http.Redirect(w, r, "/?limit", http.StatusForbidden)
			return
		}
	}

	if e := r.ParseMultipartForm(10000); e != nil {
		log.Println("Bad multipart form.", ip, r.Header.Get("Content-Type"))
		http.Redirect(w, r, "/?bad", http.StatusForbidden)
		return
	}
	// if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "multipart/form-data" {
	// 	log.Println("Not a multipart form.", ip, r.Header.Get("Content-Type"))
	// 	http.Redirect(w, r, "/?bad", http.StatusForbidden)
	// 	return
	// }

	_, fileheader, err := r.FormFile("file")
	if err != nil {
		log.Println("File read error", err)
		//	bod, _ := ioutil.ReadAll(r.Body)
		//fmt.Println(bod)

		//fmt.Println("Lnegth of req body", len(bod))
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

	// Read the file into buffer (only to cache it)
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

	// Redirect to a 320xAutoHeight thumbnail
	log.Println("Redirecting to:", "/320/0/"+id+"."+extension)
	http.Redirect(w, r, "/320/0/"+id+"."+extension, http.StatusFound)
}
