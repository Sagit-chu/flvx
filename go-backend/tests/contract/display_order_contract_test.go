package contract_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go-backend/internal/auth"
	"go-backend/internal/http/response"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func orderRequest(t *testing.T, router http.Handler, token, resource, body string) response.R {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/"+resource+"/update-order", bytes.NewBufferString(body))
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var out response.R
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, w.Body.String())
	}
	return out
}
func TestDisplayOrderUserContract(t *testing.T) {
	router, r := setupContractRouter(t, "order-secret")
	token := mustAdminToken(t, "order-secret")
	a, err := r.CreateUser("order-a", "hash", 1, 0, 10, 1, 2, 1, 3, 0)
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.CreateUser("order-b", "hash", 1, 0, 20, 1, 2, 1, 3, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := orderRequest(t, router, token, "user", fmt.Sprintf(`{"ids":[%d,%d],"expectedIds":[%d,%d]}`, a, b, b, a))
	if out.Code != 0 {
		t.Fatalf("reorder: %+v", out)
	}
	users, err := r.ListUsers()
	if err != nil {
		t.Fatal(err)
	}
	if got := []int64{users[0]["id"].(int64), users[1]["id"].(int64)}; !reflect.DeepEqual(got, []int64{a, b}) {
		t.Fatalf("order %v", got)
	}
}

func TestDisplayOrderValidationAndLifecycle(t *testing.T) {
	for _, resource := range []string{"user", "speed-limit"} {
		t.Run(resource, func(t *testing.T) {
			router, r := setupContractRouter(t, "order-secret")
			token := mustAdminToken(t, "order-secret")
			create := func(name string) int64 {
				t.Helper()
				var id int64
				var err error
				if resource == "user" {
					id, err = r.CreateUser(name, "hash", 1, 0, 10, 1, 2, 1, 3, 0)
				} else {
					id, err = r.CreateSpeedLimit(name, 100, 0, 1)
				}
				if err != nil {
					t.Fatal(err)
				}
				return id
			}
			list := func() []int64 {
				t.Helper()
				out := postBatchRequest(t, router, token, "/api/v1/"+resource+"/list", "{}")
				rows, ok := out.Data.([]interface{})
				if !ok {
					t.Fatalf("list data: %T", out.Data)
				}
				ids := []int64{}
				for _, row := range rows {
					ids = append(ids, int64(row.(map[string]interface{})["id"].(float64)))
				}
				return ids
			}
			a, b := create("a"), create("b")
			body := func(ids, expected []int64) string {
				v, _ := json.Marshal(map[string]interface{}{"ids": ids, "expectedIds": expected})
				return string(v)
			}
			check := func(payload string, code int) {
				t.Helper()
				out := orderRequest(t, router, token, resource, payload)
				if out.Code != code {
					t.Fatalf("want %d got %+v", code, out)
				}
			}
			check(body([]int64{a, b}, []int64{b, a}), 0)
			for _, payload := range []string{`{}`, `{"ids":null,"expectedIds":[]}`, `{"ids":[1.1],"expectedIds":[1]}`, body([]int64{a, a}, []int64{a, b}), body([]int64{a}, []int64{a, b}), body([]int64{a, b, 99999}, []int64{a, b}), body([]int64{a, 99999}, []int64{a, b}), body([]int64{a, b}, []int64{a, a})} {
				check(payload, 400)
			}
			if resource == "user" {
				check(body([]int64{1, a, b}, []int64{a, b}), 400)
				check(body([]int64{1, a, b}, []int64{1, a, b}), 400)
			}
			check(body([]int64{b, a}, []int64{b, a}), 409)
			c := create("c")
			if got := list(); !reflect.DeepEqual(got, []int64{c, a, b}) {
				t.Fatalf("new item first: %v", got)
			}
			check(body([]int64{b, a}, []int64{a, b}), 409)
			if resource == "user" {
				if err := r.UpdateUserWithoutPassword(a, "edited", 10, 2, 0, 1, 1, 3, 0); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := r.UpdateSpeedLimit(a, "edited", 200, 1, 0); err != nil {
					t.Fatal(err)
				}
			}
			if got := list(); !reflect.DeepEqual(got, []int64{c, a, b}) {
				t.Fatalf("edit changed order: %v", got)
			}
			if got := orderRequest(t, router, "", resource, body([]int64{b, a, c}, []int64{c, a, b})); got.Code != 401 {
				t.Fatalf("missing auth: %+v", got)
			}
			normal, err := auth.GenerateToken(a, "edited", 1, "order-secret")
			if err != nil {
				t.Fatal(err)
			}
			// Speed-limit fixture needs a real ordinary account for auth lookup.
			if resource == "speed-limit" {
				uid, err := r.CreateUser("normal", "hash", 1, 0, 10, 1, 2, 1, 3, 0)
				if err != nil {
					t.Fatal(err)
				}
				normal, err = auth.GenerateToken(uid, "normal", 1, "order-secret")
				if err != nil {
					t.Fatal(err)
				}
			}
			if got := orderRequest(t, router, normal, resource, body([]int64{b, a, c}, []int64{c, a, b})); got.Code != 403 {
				t.Fatalf("non-admin auth: %+v", got)
			}
		})
	}
}
