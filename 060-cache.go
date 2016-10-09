package main

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/drone/drone/cache"
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
