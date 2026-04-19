package socket

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func CallDashAPI(method, endpoint string, payload interface{}) error {
	client := &http.Client{Timeout: 5 * time.Second}
	var reqBody []byte
	var err error
	if payload != nil {
		reqBody, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}

	req, err := http.NewRequest(method, "http://127.0.0.1:19090"+endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("dash api error: status %d, body: %s", resp.StatusCode, string(body))
	}
	return nil
}
