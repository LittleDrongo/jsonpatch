package main

import (
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /users/{id}", getUser)
	mux.HandleFunc("PATCH /users/{id}", patchUser)
	mux.HandleFunc("PUT /users/{id}", putUser)

	addr := ":8080"
	log.Printf("listening on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
