package main

import (
	"encoding/json"
	"fmt"
)

func main() {
	var svc struct {
		Name string `json:"name"`
	}
	
	// Create mock array of objects
	data := []map[string]interface{}{
		{"name": "test1"},
	}
	jsonData, _ := json.Marshal(data)
	
	err := json.Unmarshal(jsonData, &svc)
	fmt.Printf("Error array to struct: %v\n", err)
	
	var svcs []struct {
		Name string `json:"name"`
	}
	err = json.Unmarshal(jsonData, &svcs)
	fmt.Printf("Error array to array: %v\n", err)
	if err == nil && len(svcs) > 0 {
		fmt.Printf("Name: %s\n", svcs[0].Name)
	}
}
