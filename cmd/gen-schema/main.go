// Command gen-schema prints the OpenAPI 3.1 document of the API as JSON.
package main

import (
	"encoding/json"
	"log"
	"os"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/config"
)

func main() {
	_, humaAPI := api.New(nil, config.Config{}, api.Deps{})
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(humaAPI.OpenAPI()); err != nil {
		log.Fatal(err)
	}
}
