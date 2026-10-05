package main

import (
	"encoding/json"
	"fmt"
	"log"

	"app/jsonpatch"
)

type Profile struct {
	ID    int    `json:"id" jsonpatch:"readonly"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role" jsonpatch:"readonly"`
}

func main() {
	SampleFullModelMerge()
	SampleDTOAndFullModelMerge()
}

// SampleFullModelMerge uses tags on the full model to protect read-only fields.
func SampleFullModelMerge() {
	user := Profile{ID: 7, Name: "Alice", Email: "alice@example.com", Role: "admin"}
	patch := []byte(`{"id":99,"name":"Bob","role":"user","unknown":"ignored"}`)

	if err := jsonpatch.Merge(&user, patch); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Full model merge: %+v\n", user)
}

// SampleDTOAndFullModelMerge uses a small DTO as the patch allowlist.
func SampleDTOAndFullModelMerge() {

	// UserDTO contains the fields and values this handler accepts for an update.
	type UserDTO struct {
		Name  *string `json:"name"`
		Email *string `json:"email"`
		Role  *string `json:"role"`
	}

	user := Profile{ID: 7, Name: "Alice", Email: "alice@example.com", Role: "admin"}
	body := []byte(`{"name":"Bob","email":"bob@example.com","role":"user"}`)
	var dto UserDTO
	if err := json.Unmarshal(body, &dto); err != nil {
		log.Fatal(err)
	}

	if err := jsonpatch.MergeFrom(&user, &dto); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("DTO and full model merge: %+v\n", user)
}
