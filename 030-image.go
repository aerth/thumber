package main

import (
	"fmt"
	"image"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aerth/filer"
	"github.com/gorilla/mux"
)

// If a file is an image, this returns the image.Image of the file.
func getimage(id string) image.Image {
	reader, err := os.Open(*uploadsDir + id)
	if err != nil {
		return nil
	}
	defer reader.Close()
	m, s, err := image.Decode(reader)
	if err != nil {
		log.Println(err)
		return nil
	}
	log.Println("Read Image:", s, *uploadsDir+id[:6])
	return m
}

// Just read a file
func getbytes(id string) []byte {
	b, err := ioutil.ReadFile(*uploadsDir + id)
	if err != nil {
		log.Println(err)
		return nil
	}
	return b
}

// Serve route
func serve(route *mux.Router) {
	e := http.ListenAndServe(*netint+":"+*port, route)
	if e != nil {
		fmt.Println("Error:", e)
		os.Exit(2)
	}
}

// Limiting struct
type Limiting struct {
	Since       time.Time
	Until       time.Time
	RateLimited bool
	Count       int
}

// Generate random string
func keygen(n int) string {
	runes := []rune("abcdefg1234567890123456789012345678901234567890")
	b := make([]rune, n)
	for i := range b {
		b[i] = runes[rand.Intn(len(runes))]
	}
	return strings.TrimSpace(string(b))
}

// Make sure keygen is unique file
func unique() string {
	id := keygen(*filenameLength)
	_, er := os.Open(id)
	if er != nil {
		if strings.Contains(er.Error(), "no such file or directory") {
			filer.Touch("./uploads/" + id)
			return id
		}
		log.Println(er)
	}
	return unique()
}
