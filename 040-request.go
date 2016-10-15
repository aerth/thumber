package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"strings"
	"sync"
	"time"
)

var mutex = new(sync.Mutex)

// LogLiner listens for requests to come in and formats them into a log line.
func logs() {
	var totalhits int
	for {
		t1 := time.Now()
		// Receive request
		l := <-logchan

		t2 := time.Now()

		// t0 = request total time
		t0 := t2
		t3 := t2.Sub(t1)
		if *debug {
			log.Println("Been waiting for new request for: ", t3)
		}
		t1 = t2
		// Just want IP
		ip := strings.Split(l.RemoteAddr, ":")[0]
		// Increment total hit counter
		totalhits++

		// POST weighs more
		var weight = 1
		if l.Method == "POST" {
			weight = 5
		}

		// pass through
		var pass bool
		t2 = time.Now()
		t3 = t2.Sub(t1)
		if *debug {
			log.Println("Been whatever for", t3)
		}
		t1 = time.Now()
		// Read map
		if *debug {
			log.Println("Locking for RateLimiter read")
		}
		mutex.Lock()
		user := visitor[ip]
		mutex.Unlock()
		if *debug {
			log.Println("UnLocking for RateLimiter write")
		}
		if user == nil {
			if *debug {
				log.Println("New User", ip)
			}
			user = &Limiting{Since: time.Now()}
			pass = true
		}
		t2 = time.Now()
		t3 = t2.Sub(t1)
		if *debug {
			log.Println("Been reading map for", t3)
		}
		if *noratelimiting {
			pass = true
		}
		if !pass {
			// If Since is before 10 seconds ago, reset counter.. Unless user is rate limited.
			// If user is rate limited, reset counter after Until happens
			if (!user.RateLimited && user.Since.Before(time.Now().Add(-10*time.Second))) || (user.RateLimited && user.Until.Before(time.Now())) {
				user.Since = time.Now()
				user.Count = 0
				user.Until = time.Time{} // IsZero
				user.RateLimited = false
			} else {
				user.Count = user.Count + weight // Normal second, third (and so on) request
			}

			// Over speed limit (15/sec). Wait 1 more seconds.
			if user.Count > 15 {
				if *debug {
					log.Println("RateLimit User:", ip)
				}
				user.RateLimited = true
				// Until is now
				if user.Until.IsZero() {
					user.Until = time.Now()
				}
				// Until is 5 seconds from Until
				user.Until = user.Until.Add(1 * time.Second) // Keep incrementing if they keep trying
			}

		}

		// Write to map
		if *debug {
			log.Println("Locking for RateLimiter write")
		}
		mutex.Lock()
		visitor[ip] = user
		mutex.Unlock()
		if *debug {
			log.Println("UnLocking RateLimiter write")
		}
		// log the request (no map lookup in log formatter for panic risk)
		s := fmt.Sprintf("%v (%+003v) %s %q %q > %q %q", ip, user, l.Method, l.RequestURI, l.UserAgent(), l.RemoteAddr, l.Host)
		if l.Referer() != "" {
			s += "ref: " + l.Referer()
		}
		if *debug {
			log.Println(s)
			s, _ := ioutil.ReadAll(l.Body)
			str := string(s)
			log.Println("request body:", len(str), l.URL.Path)

		}
		t2 = time.Now()
		if *debug {
			log.Println("Log function took:", t2.Sub(t0))
		}
	}
}
