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
	m, _, err := image.Decode(reader)
	if err != nil {
		log.Println(err)
		return nil
	}
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

// LogLiner listens for requests to come in and formats them into a log line.
func logs() {
	var totalhits int
	for {
		l := <-logchan
		ip := strings.Split(l.RemoteAddr, ":")[0]
		lim[ip]++
		totalhits++
		if visitor[ip] == 0 {
			visitor[ip] = totalhits
		}
		s := fmt.Sprintf("%v (%+003v) #%+003v %s %q %q > %q %q", ip, visitor[ip], lim[ip], l.Method, l.RequestURI, l.UserAgent(), l.RemoteAddr, l.Host)
		if l.Referer() != "" {
			s += "ref: " + l.Referer()
		}
		log.Println(s)
	}
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
	id := keygen(10)
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
