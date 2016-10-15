package main

import (
	"image"
	"io/ioutil"
	"log"
	"os"
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
func getbytes(id string) ([]byte, error) {
	b, err := ioutil.ReadFile(*uploadsDir + id)
	if err != nil {

		return nil, err
	}
	return b, nil
}
