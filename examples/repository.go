package main

import (
	"net/http"
	"strconv"
	"sync"
)

// users is a tiny in-memory repository.
var users = struct {
	sync.RWMutex
	m map[int]profile
}{
	m: map[int]profile{
		1: {ID: 1, Name: "Alice", Email: "alice@example.com", Role: "admin"},
		2: {ID: 2, Name: "Bob", Email: "bob@example.com", Role: "user"},
	},
}

func loadUser(id int) (profile, bool) {
	users.RLock()
	user, found := users.m[id]
	users.RUnlock()
	return user, found
}

func saveUser(id int, user profile) {
	users.Lock()
	users.m[id] = user
	users.Unlock()
}

func pathID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}
