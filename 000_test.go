package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

/*

Test that:
⎈ home page gets up, testserver can start
⎈ invalid requests get trashed
valid requests are valid
valid requests get cached
cache gets invalidated after x
limit kicks in after 20 req / 10 sec

limiter rules:
upload is scored as 5 req
view/resize is 1 req
cached is 0 req

make sure this is valid:
  upload 2 files, view 2 files, resize 2 files in ten seconds
  (total 6 clicks, 14 reqs ,under 20)

*/

var ts *httptest.Server

const tmpdir = "tmpTestDirDeleteMe/"

func init() {
	//log.SetPrefix("\n\nts>\t")
	log.SetFlags(log.Lshortfile)
	log.SetPrefix("")
	*uploadsDir = tmpdir
	os.Mkdir(tmpdir, 0777)
	// Log requests
	*port = "9999"
	go logs()

	// Limit hit per IP per second
	go ratelimiter()

	// Serve
	ts = httptest.NewServer(r)

}

func TestHome(t *testing.T) {

	rs, e := http.Get(ts.URL)
	if e != nil {
		t.FailNow()
		fmt.Println(e)
	}
	resp, _ := ioutil.ReadAll(rs.Body)
	rs.Body.Close()
	//fmt.Println(string(resp))

	if string(resp) != homegold {
		t.Fail()
		fmt.Println("Got:", string(resp))
		fmt.Println("Wanted:", homegold)
	}
}
func TestRedirectHome(t *testing.T) {
	invalidgroup := []string{"/longlength.png", "/short.png", "/index.php", "/⚛",
		"/phpMyAdmin/index.php", "/somethingrandom", "/funny.js", "/1/2/3"}
	for _, testcase := range invalidgroup {
		logbuf := new(bytes.Buffer)
		log.SetOutput(logbuf)
		fmt.Println("[TestRedirectHome] Trying:", testcase)
		// Not Found
		if testcase == "" {
			continue
		}

		req := httptest.NewRequest("GET", ts.URL+testcase, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.True(t, strings.Contains(logbuf.String(), "Home Redirecting"))
		// Test StatusCode
		if w.Code != 302 {
			fmt.Println(testcase, w.Code, "!= 302")
			t.FailNow()
			continue
		}
		// Test Body (404 page not found)

		if w.Body.String() != "<a href=\"/\">Found</a>.\n\n" {
			fmt.Println("testcase:", testcase)
			fmt.Printf("\tGot: %q\n\tWant: %q\n\n", w.Body.String(), "<a href=\"/\">Found</a>.\n\n")
			t.Fail()
			continue
		}
	}
}
func TestBadForm(t *testing.T) {
	// Redirect server logs temporarily
	var logbuf = new(bytes.Buffer)
	log.SetOutput(logbuf)

	// Form data is a buffer
	var formdata = new(bytes.Buffer)
	formdata.WriteString("hello")
	req, _ := http.NewRequest("POST", ts.URL+"/upload", ioutil.NopCloser(formdata))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	found := w.Header().Get("Location")
	if w.Code != 403 || found != "/?bad" || w.Body.String() != "" || !strings.Contains(logbuf.String(), "Bad multipart form.") {
		fmt.Println("Found:", found)
		fmt.Println("Body:", w.Body.String())
		fmt.Println(w.Code)
		fmt.Println("Log:", logbuf.String())
		t.Fail()
	}
}
func TestImgUpload(t *testing.T) {

	filename := "testdata/wu.jpg"
	target := ts.URL + "/upload"
	//
	// // file (io.Reader)
	// f, err := os.Open(filename)
	// assert.Nil(t, err)

	// load the filebytes
	picbuf, err := ioutil.ReadFile(filename)
	assert.Nil(t, err)

	// request body
	body := new(bytes.Buffer)

	// multipart writer formats the body
	ww := multipart.NewWriter(body)
	assert.Nil(t, err)
	formWriter, err := ww.CreateFormFile("file", "null.jpg")
	assert.Nil(t, err)
	formWriter.Write(picbuf)
	if err = ww.Close(); err != nil {
		t.Fatal(err)
	}

	// Create request
	req, err := http.NewRequest("POST", target, body)
	req.Header.Add("Content-Type", ww.FormDataContentType())
	assert.Nil(t, err)

	// Tester (r is mux Router)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Check response
	// (Should be something like: /320/0/d15454.jpg) where d15454 is the generated ID
	found := w.Header().Get("Location")
	fmt.Println("[TestImgUpload] Found:", found)
	slash := strings.Split(strings.TrimPrefix(found, "/"), "/")

	if found == "/?limit" {
		fmt.Println("[TestImgUpload] Limited")
		return
	}
	assert.Equal(t, "320", slash[0])
	assert.Equal(t, slash[1], "0")
	assert.True(t, strings.HasSuffix(slash[2], ".jpg"))
	newtarget := ts.URL + "/" + slash[2]

	// Try the get the image
	fmt.Println("[TestImgUpload] Trying: ", newtarget)
	br, be := http.Get(newtarget)
	assert.Nil(t, be)
	bb, be := ioutil.ReadAll(br.Body)
	assert.Nil(t, be)

	// Test lengths
	assert.Equal(t, len(picbuf), len(bb))

	// Test bytes.Equal
	if !bytes.Equal(picbuf, bb) {
		t.Fail()
	}
	if !t.Failed() {
		fmt.Println("[TestImgUpload] Success!", newtarget)
	}
	goodImageURL = newtarget
}

var goodImageURL string

func TestImgCached(t *testing.T) {
	if goodImageURL == "" {
		TestImgUpload(t)
	}
	logbuf := new(bytes.Buffer)
	log.SetOutput(logbuf)
	defer log.SetOutput(os.Stdout)
	br, be := http.Get(goodImageURL)
	assert.Nil(t, be)
	bb, be := ioutil.ReadAll(br.Body)
	assert.Nil(t, be)
	assert.True(t, strings.Contains(logbuf.String(), "is cached."))
	b, _ := ioutil.ReadFile("testdata/wu.jpg")
	assert.Equal(t, b, bb)
}
func TestImgTrashed(t *testing.T) {
	logbuf := new(bytes.Buffer)
	log.SetOutput(logbuf)

	// We look for a line thats only available in -debug mode
	booly := *debug
	*debug = true
	defer func() {
		// Reset debug flag after this test
		*debug = booly
	}()
	defer log.SetOutput(os.Stdout)

	br, be := http.Get(ts.URL + "/00XX00.jpg") // most likely not created yet
	assert.Nil(t, be)
	bb, be := ioutil.ReadAll(br.Body)
	assert.Nil(t, be)

	if booly {
		fmt.Println("[TestImgTrashed] Log Buffer: ", logbuf.String())
	}
	assert.True(t, strings.Contains(logbuf.String(), "Image not found"))
	if *debug {
	}

	assert.Equal(t, br.Request.URL.Path, "/")
	assert.Equal(t, homegold, string(bb))
	if !t.Failed() {
		fmt.Println("[TestImgTrashed] Successful Redirect")
	}
}

func TestLimitBorder(t *testing.T) {

	logbuf := new(bytes.Buffer)
	log.SetOutput(logbuf)
	defer log.SetOutput(os.Stdout)
	var wg sync.WaitGroup

	for i := 0; i < 4; i++ {

		wg.Add(1)
		time.Sleep(1000 * time.Millisecond)
		go func() {
			fmt.Println("Uploading and Viewing 1") // upload : 5 view : 1 = 6
			TestImgUpload(t)
			wg.Done()
		}()

		fmt.Println("Waiting 10 seconds and uploading another")
		time.Sleep(10 * time.Second)
	}

	wg.Wait()
	// shouldn't take too long :D

	assert.False(t, strings.Contains(logbuf.String(), "Not serving, rate limited"))

}

func TestLimit(t *testing.T) {
	logbuf := new(bytes.Buffer)
	log.SetOutput(logbuf)
	defer log.SetOutput(os.Stdout)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			TestImgUpload(t)
			wg.Done()
		}()
	}
	wg.Wait()
	// shouldn't take too long :D

	assert.True(t, strings.Contains(logbuf.String(), "Not serving, rate limited"))

}

var homegold = `<!DOCTYPE html>
<html>
<head>
	<meta http-equiv="Content-Type" content="text/html; charset=utf-8">
	<meta name="viewport" content="width=device-width">
	<meta name="theme-color" content="#375EAB">
	<title>Thumber v1</title>
<style>
body{
  color: green;
  background-color:   #E0EBF5;
}
.box {
border: 1px solid black;
min-width: 100px;
margin: 10px;
float: left;
clear: none;
padding: 10px;
}
</style>
<body>


<h1>Thumber</h1>
<h2>Thumbnail Server</h2>
<h3> Upload a file </h3>
<form id="post" action="/upload" enctype="multipart/form-data" method="POST">
		<input name="file" type="file" required/></input>
    <br><input id="upload-submit" type="submit" value="upload" />
</form>
<pre style="background-color: lightgrey; width: 300px;">
Public API:

Original Size: /fileID
Resize: /width/height/fileID
Resize: /fileID/width/height (alt)
Upload: POST /upload

Example: /640/480/cat.jpeg

</pre>
</body></html>`
