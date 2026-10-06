package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/LittleDrongo/jsonpatch"
)

type profile struct {
	ID    int    `json:"id" jsonpatch:"readonly"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role" jsonpatch:"readonly"`
}

var mergeOptions = jsonpatch.Options{ErrorOnReadOnly: true}

func getUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	user, found := loadUser(id)
	if !found {
		http.NotFound(w, r)
		return
	}

	log.Printf("users/getUser: %v\n", user)
	writeJSON(w, http.StatusOK, user)
}

func patchUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}

	user, found := loadUser(id)
	if !found {
		http.NotFound(w, r)
		return
	}

	merged, err := jsonpatch.Merge(&user, body, mergeOptions)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	log.Printf("users/patchUser: %v\n", merged)
	saveUser(id, merged)
	writeJSON(w, http.StatusOK, merged)
}

func putUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}

	user, found := loadUser(id)
	if !found {
		http.NotFound(w, r)
		return
	}

	merged, err := jsonpatch.Merge(&user, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	saveUser(id, merged)
	log.Printf("users/putUser: %v\n", merged)
	writeJSON(w, http.StatusOK, merged)
}

// ------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}
