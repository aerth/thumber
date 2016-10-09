package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/drone/drone/cache"
	"github.com/gorilla/mux"
)

var (
	port           = flag.String("port", "8081", "Port to serve on")
	netint         = flag.String("bind", "127.0.0.1", "Interface to bind to")
	uploadsDir     = flag.String("up", "uploads", "Directory to save uploaded files")
	debug          = flag.Bool("v", false, "Enable logs")
	noratelimiting = flag.Bool("swamped", false, "Disable rate limiting")
	perm           = flag.Int("perm", 0700, "Permissions for uploads directory")
	maxusers       = flag.Int("max", 1, "Max users at one time")
	filenameLength = flag.Int("len", 100, "File ID length")
	customFormat   = flag.String("custom", "", "Custom formatting."+formathelp)
	version        = "Thumber v1"
	formathelp     = `

Here are the default URL formats.
Your alternative one NEEDS 'w', 'h', and 'id' to work.
[For /width/height/id formatting:]
	/{w:[0-9]+}/{h:[0-9]+}/{id}

[For /id/width/height formatting:]
	/{id}/{w:[0-9]+}/{h:[0-9]+}

[Try something like:]
	/thumb/{w:[0-9]+}/{h:[0-9]+}/{id}


See gorilla/mux HandleFunc formatting for more information.

	`
)

var r *mux.Router

func init() {
	of := flag.Usage
	rand.Seed(time.Now().UnixNano())
	flag.Usage = func() {
		fmt.Println(version)
		fmt.Println("A thumbnail server")
		of()
	}
	if !strings.Contains(*uploadsDir, "/") {
		*uploadsDir = "./" + *uploadsDir + "/"
	}
	os.Mkdir(*uploadsDir, os.FileMode(uint32(*perm)))
	r = mux.NewRouter()
	r.HandleFunc("/", s0Home).Methods("GET")
	r.HandleFunc("/upload", s0Upload).Methods("POST")
	r.HandleFunc("/{w:[0-9]+}/{h:[0-9]+}/{id}", s0Resize).Methods("GET")
	r.HandleFunc("/{id}/{w:[0-9]+}/{h:[0-9]+}", s0Resize).Methods("GET")
	if *customFormat != "" {
		r.HandleFunc(*customFormat, s0Resize).Methods("GET")
	}

	r.HandleFunc("/{id}", s0Get).Methods("GET")
	http.Handle("/", r)
	c1 = cache.Default()
}

func main() {
	flag.Parse()

	if len(flag.Args()) != 0 {
		flag.Usage()
		os.Exit(2)
	}
	if *debug {
		log.SetFlags(log.Llongfile)
	}
	go func() {
		time.Sleep(400 * time.Millisecond)
		log.Printf("Serving on %s:%s", *netint, *port) // go func in case port is unavailable
	}()
	go logs()
	go ratelimiter()
	serve(r)

}
