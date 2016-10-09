package main

import (
	"bytes"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"log"
)

const (
	// PNG 0
	PNG = iota
	// JPG 1
	JPG
	// GIF 2
	GIF
	// JPEG 3
	JPEG = JPG
)

// Imagething is an image.Image suspended in memory
type Imagething struct {
	buf bytes.Buffer
	ext string
}

// DetectFormat of a user submitted  image file fastest
func DetectFormat(buf bytes.Buffer, newImage image.Image) chan Imagething {
	imagechan := make(chan Imagething)
	go func() {
		// Test for PNG
		var buf bytes.Buffer
		pngError := png.Encode(&buf, newImage)
		if pngError != nil {
			log.Println(pngError)
		} else {

			imagechan <- Imagething{buf: buf, ext: "png"}
		}
	}()
	go func() {
		// Test for JPEG
		var buf bytes.Buffer
		jpegError := jpeg.Encode(&buf, newImage, nil)
		if jpegError != nil {
			log.Println(jpegError)
		} else {
			imagechan <- Imagething{buf: buf, ext: "jpg"}
		}
	}()
	go func() {
		// Test for GIF
		var buf bytes.Buffer
		gifError := gif.Encode(&buf, newImage, nil)
		if gifError != nil {
			log.Println(gifError)
		} else {
			imagechan <- Imagething{buf: buf, ext: "gif"}
		}
	}()
	return imagechan
}
