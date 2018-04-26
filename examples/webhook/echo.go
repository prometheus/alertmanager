package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
)

func main() {
	http.ListenAndServe(":5003", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}
		defer r.Body.Close()
		var buf bytes.Buffer
		if err := json.Indent(&buf, b, " >", "  "); err != nil {
			panic(err)
		}
		log.Println(buf.String())
	}))
}
