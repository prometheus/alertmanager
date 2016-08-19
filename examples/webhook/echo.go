package main

import (
	"io/ioutil"
	"log"
	"net/http"
)

func main() {
	http.ListenAndServe(":5001", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println("received request:", r.Header)
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}
		defer r.Body.Close()
		log.Println("request body:", string(b))
	}))
}
