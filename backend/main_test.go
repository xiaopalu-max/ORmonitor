package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestCoreAPICompatibility(t *testing.T) {
	t.Parallel()

	st, err := newStore(filepath.Join(t.TempDir(), "db.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	s := &server{
		store:    st,
		sessions: make(map[string]session),
		httpClient: &http.Client{
			Timeout: time.Second,
		},
	}

	rr := performJSON(s, http.MethodGet, "/api/keys", nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized keys status = %d", rr.Code)
	}

	login := performJSON(s, http.MethodPost, "/api/auth/login", map[string]any{
		"username": "admin",
		"password": "admin123",
	})
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", login.Code, login.Body.String())
	}
	var loginBody map[string]any
	if err := json.Unmarshal(login.Body.Bytes(), &loginBody); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	sessionID := stringValue(loginBody["sessionId"])
	if sessionID == "" {
		t.Fatal("missing sessionId")
	}

	create := performJSON(s, http.MethodPost, "/api/keys?sessionId="+sessionID, map[string]any{
		"name":  "primary",
		"key":   "sk-test",
		"quota": 12.5,
		"group": "ops",
	})
	if create.Code != http.StatusOK {
		t.Fatalf("create key status = %d body=%s", create.Code, create.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created key: %v", err)
	}
	keyID := stringValue(created["id"])
	if keyID == "" || stringValue(created["group"]) != "ops" || numberValue(created["quota"]) != 12.5 {
		t.Fatalf("unexpected created key: %#v", created)
	}

	update := performJSON(s, http.MethodPut, "/api/keys/"+keyID+"?sessionId="+sessionID, map[string]any{
		"name":             "primary-updated",
		"key":              "sk-test",
		"quota":            20,
		"group":            "ops",
		"warningThreshold": 5,
	})
	if update.Code != http.StatusOK {
		t.Fatalf("update key status = %d body=%s", update.Code, update.Body.String())
	}

	archive := performJSON(s, http.MethodPost, "/api/keys/"+keyID+"/archive?sessionId="+sessionID, nil)
	if archive.Code != http.StatusOK {
		t.Fatalf("archive status = %d body=%s", archive.Code, archive.Body.String())
	}
	unarchive := performJSON(s, http.MethodDelete, "/api/keys/"+keyID+"/archive?sessionId="+sessionID, nil)
	if unarchive.Code != http.StatusOK {
		t.Fatalf("unarchive status = %d body=%s", unarchive.Code, unarchive.Body.String())
	}

	settings := performJSON(s, http.MethodPost, "/api/settings?sessionId="+sessionID, map[string]any{
		"enableNotify":   false,
		"notifyInterval": 1,
		"purchaseRate":   3.5,
		"sellRate":       4.0,
		"notifyChannels": map[string]any{},
	})
	if settings.Code != http.StatusOK {
		t.Fatalf("settings status = %d body=%s", settings.Code, settings.Body.String())
	}

	history := performJSON(s, http.MethodPost, "/api/balance-history?sessionId="+sessionID, map[string]any{
		"keyId":     "all",
		"balance":   20,
		"usage":     1,
		"remaining": 19,
	})
	if history.Code != http.StatusOK {
		t.Fatalf("history create status = %d body=%s", history.Code, history.Body.String())
	}
	listHistory := performJSON(s, http.MethodGet, "/api/balance-history?sessionId="+sessionID+"&keyId=all", nil)
	if listHistory.Code != http.StatusOK {
		t.Fatalf("history list status = %d body=%s", listHistory.Code, listHistory.Body.String())
	}
	var historyRows []map[string]any
	if err := json.Unmarshal(listHistory.Body.Bytes(), &historyRows); err != nil {
		t.Fatalf("decode history rows: %v", err)
	}
	if len(historyRows) != 1 || stringValue(historyRows[0]["keyId"]) != "all" {
		t.Fatalf("unexpected history rows: %#v", historyRows)
	}
}

func performJSON(handler http.Handler, method, target string, body map[string]any) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, target, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}
