package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/proof", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "secure proof")
	})
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
