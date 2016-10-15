package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aerth/filer"
	"github.com/drone/drone/cache"
	"github.com/gorilla/mux"
)

var (
	timing         = flag.Duration("timing", time.Minute*3, "Interval to reset cache. 0 to disable.")
	port           = flag.String("port", "8081", "Port to serve on")
	netint         = flag.String("bind", "127.0.0.1", "Interface to bind to")
	logfile        = flag.String("log", "debug.log", "Log file")
	uploadsDir     = flag.String("up", "uploads", "Directory to save uploaded files")
	debug          = flag.Bool("debug", false, "Enable logs")
	noratelimiting = flag.Bool("swamped", false, "Disable rate limiting")
	perm           = flag.Int("perm", 0700, "Permissions for uploads directory")
	maxusers       = flag.Int("max", 1, "Max users at one time")
	filenameLength = flag.Int("len", 6, "File ID length")
	customFormat   = flag.String("custom", "", "Custom formatting."+formathelp)
	version        = "Thumber v1"
	formathelp     = `

Here are the default URL formats. regexp is ok.
Your alternative format NEEDS 'w', 'h', 'id', and 'ext' to work.

[Try something like:]  http://localhost:8083/thumb/300/400/abc123.png

	/thumb/{w:[0-9]+}/{h:[0-9]+}/{id}.{ext}

[Or] http://localhost:8083/png/300/400/abc123

	/{ext}/{w:[0-9]+}/{h:[0-9]+}/{id}



See gorilla/mux HandleFunc formatting for more information.

	`
)

var r *mux.Router

func init() {

	// Random
	rand.Seed(time.Now().UnixNano())

	// Redefine flag.Usage()
	of := flag.Usage
	flag.Usage = func() {
		fmt.Println(version)
		fmt.Println("A thumbnail server")
		of()
	}

	// Format uploadsDir
	if !strings.Contains(*uploadsDir, "/") {
		*uploadsDir = "./" + *uploadsDir + "/"
	}

	// Create the uploadsDir
	e := os.Mkdir(*uploadsDir, os.FileMode(uint32(*perm)))
	if e != nil {
		if !strings.Contains(e.Error(), "exists") {
			log.Fatalln(e)
		}
	}

	e = filer.Touch(*uploadsDir + "boot")
	if e != nil {
		if !strings.Contains(e.Error(), "exists") {
			log.Fatalln(e)
		}
	}
	// URL routing
	r = mux.NewRouter()

	r.HandleFunc("/upload", s0Upload).Methods("POST")

	if *customFormat != "" {
		r.HandleFunc(*customFormat, s0ResizeExt).Methods("GET")
	}

	r.HandleFunc("/{w:[0-9]+}/{h:[0-9]+}/{id}.{ext}", s0ResizeExt).Methods("GET")
	r.HandleFunc("/{id}.{ext}/{w:[0-9]+}/{h:[0-9]+}", s0ResizeExt).Methods("GET")
	r.HandleFunc("/{id:[a-zA-Z0-9]{"+strconv.Itoa(*filenameLength)+"}}.{ext:jpg|jpeg|png|gif}",
		s0Get).Methods("GET")
	// r.HandleFunc("/{id}.{ext:jpeg}", s0Get).Methods("GET")
	// r.HandleFunc("/{id}.{ext:gif}", s0Get).Methods("GET")
	r.HandleFunc("/", s0Home)
	//r.HandleFunc("/{whatever}", s0Home)
	//	r.HandleFunc("/{what}.{ever}", s0Home)
	r.NotFoundHandler = http.HandlerFunc(s0Home)
	http.Handle("/", r)

	// New Cache
	c1 = cache.NewTTL(*timing)

}

func main() {
	flag.Parse()

	if len(flag.Args()) != 0 {
		flag.Usage()
		os.Exit(2)
	}

	// Filename + Line numbers
	if *debug {
		log.SetFlags(log.Llongfile)
	}
	// Log to debug.log
	if *logfile == "stdout" {
		log.SetOutput(os.Stdout)
	} else {
		debuglog, e := os.OpenFile(*logfile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0750)
		if e != nil {
			panic(e)
		}
		log.SetOutput(debuglog)
	}

	// Notify user we are serving
	go func() {
		time.Sleep(400 * time.Millisecond)
		fmt.Printf("Serving on %s:%s\n", *netint, *port) // go func in case port is unavailable
		fmt.Printf("Logging to %q\n", *logfile)          // go func in case port is unavailable
		return
	}()

	// Log requests
	go logs()

	// Limit hit per IP per second
	go ratelimiter()

	// Serve
	serve(r)

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
