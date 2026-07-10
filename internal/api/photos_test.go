package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/jimgcampbell/food/internal/food"
)

type fakePhotoStore struct {
	uploadErr   error
	uploadedCT  string
	uploadedKey string

	getData []byte
	getCT   string
	getErr  error
	getKey  string
}

func (f *fakePhotoStore) UploadPhoto(ctx context.Context, key, contentType string, data io.Reader) (string, error) {
	if f.uploadErr != nil {
		return "", f.uploadErr
	}
	f.uploadedKey = key
	f.uploadedCT = contentType
	if _, err := io.Copy(io.Discard, data); err != nil {
		return "", err
	}
	return "https://cdn.example.com/" + key, nil
}

func (f *fakePhotoStore) GetPhoto(ctx context.Context, key string) ([]byte, string, error) {
	f.getKey = key
	if f.getErr != nil {
		return nil, "", f.getErr
	}
	return f.getData, f.getCT, nil
}

type fakeImageParser struct {
	imageData []byte
	mediaType string
	hint      string
	day       string
	result    *food.ParseResult
	err       error
}

func (f *fakeImageParser) ParseImage(ctx context.Context, imageData []byte, mediaType, hint, day string, progress func(string)) (*food.ParseResult, error) {
	f.imageData = imageData
	f.mediaType = mediaType
	f.hint = hint
	f.day = day
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func multipartImageRequest(t *testing.T, fieldName, filename, contentType string, data []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", `form-data; name="`+fieldName+`"; filename="`+filename+`"`)
	if contentType != "" {
		partHeader.Set("Content-Type", contentType)
	}
	part, err := w.CreatePart(partHeader)
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/photos", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func TestPhotoUploadHappyPath(t *testing.T) {
	store := &fakePhotoStore{}
	h := NewPhotoHandler(store, nil, slog.Default())

	req := multipartImageRequest(t, "image", "meal.jpg", "image/jpeg", []byte("fake-jpeg-bytes"))
	rr := httptest.NewRecorder()
	h.upload(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Key string `json:"key"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.HasPrefix(resp.Key, "food/") || !strings.HasSuffix(resp.Key, ".jpg") {
		t.Errorf("key = %q, want food/.../*.jpg", resp.Key)
	}
	if resp.URL != "https://cdn.example.com/"+resp.Key {
		t.Errorf("url = %q, want it to match the uploaded key", resp.URL)
	}
	if store.uploadedCT != "image/jpeg" {
		t.Errorf("uploaded content type = %q, want image/jpeg", store.uploadedCT)
	}
}

func TestPhotoUploadRejectsBadContentType(t *testing.T) {
	store := &fakePhotoStore{}
	h := NewPhotoHandler(store, nil, slog.Default())

	req := multipartImageRequest(t, "image", "meal.gif", "image/gif", []byte("gif-bytes"))
	rr := httptest.NewRecorder()
	h.upload(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	if store.uploadedKey != "" {
		t.Error("store should not have been called for a rejected content type")
	}
}

func TestPhotoUploadRejectsOversize(t *testing.T) {
	store := &fakePhotoStore{}
	h := NewPhotoHandler(store, nil, slog.Default())

	big := bytes.Repeat([]byte{'a'}, maxPhotoBytes+1)
	req := multipartImageRequest(t, "image", "meal.jpg", "image/jpeg", big)
	rr := httptest.NewRecorder()
	h.upload(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an oversize image; body: %s", rr.Code, rr.Body.String())
	}
	if store.uploadedKey != "" {
		t.Error("store should not have been called for an oversize image")
	}
}

func TestPhotoUpload503WhenR2Unconfigured(t *testing.T) {
	h := NewPhotoHandler(nil, nil, slog.Default())

	req := multipartImageRequest(t, "image", "meal.jpg", "image/jpeg", []byte("fake"))
	rr := httptest.NewRecorder()
	h.upload(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body: %s", rr.Code, rr.Body.String())
	}
}

func TestAnalyzePhotoPassesImageBytesToParser(t *testing.T) {
	store := &fakePhotoStore{getData: []byte("photo-bytes"), getCT: "image/png"}
	parser := &fakeImageParser{result: &food.ParseResult{Notes: "parsed from photo"}}
	h := NewPhotoHandler(store, parser, slog.Default())

	body, _ := json.Marshal(analyzePhotoRequest{Key: "food/2026/07/abc.png", Hint: "I had half of this", Day: "2026-07-07"})
	req := httptest.NewRequest(http.MethodPost, "/analyze-photo", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.analyze(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	if store.getKey != "food/2026/07/abc.png" {
		t.Errorf("fetched key = %q, want the requested key", store.getKey)
	}
	if string(parser.imageData) != "photo-bytes" {
		t.Errorf("parser received image data %q, want %q", parser.imageData, "photo-bytes")
	}
	if parser.mediaType != "image/png" {
		t.Errorf("parser media type = %q, want image/png", parser.mediaType)
	}
	if parser.hint != "I had half of this" {
		t.Errorf("parser hint = %q, want the request hint", parser.hint)
	}
	if parser.day != "2026-07-07" {
		t.Errorf("parser day = %q, want 2026-07-07", parser.day)
	}

	// The response is an NDJSON stream: progress event(s), then the result.
	if ct := rr.Header().Get("Content-Type"); ct != "application/x-ndjson" {
		t.Errorf("Content-Type = %q, want application/x-ndjson", ct)
	}
	lines := strings.Split(strings.TrimSpace(rr.Body.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("stream has %d lines, want at least a progress and a result event: %s", len(lines), rr.Body.String())
	}
	var last struct {
		Type   string           `json:"type"`
		Result food.ParseResult `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &last); err != nil {
		t.Fatalf("decode final stream event: %v", err)
	}
	if last.Type != "result" {
		t.Fatalf("final event type = %q, want result", last.Type)
	}
	if last.Result.Notes != "parsed from photo" {
		t.Errorf("Notes = %q, want the parser's result echoed back", last.Result.Notes)
	}
}

func TestAnalyzePhoto503WhenUnconfigured(t *testing.T) {
	body, _ := json.Marshal(analyzePhotoRequest{Key: "k", Day: "2026-07-07"})

	cases := []struct {
		name   string
		store  PhotoStore
		parser ImageParser
	}{
		{"no store", nil, &fakeImageParser{}},
		{"no parser", &fakePhotoStore{}, nil},
		{"neither", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewPhotoHandler(tc.store, tc.parser, slog.Default())
			req := httptest.NewRequest(http.MethodPost, "/analyze-photo", bytes.NewReader(body))
			rr := httptest.NewRecorder()
			h.analyze(rr, req)
			if rr.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503; body: %s", rr.Code, rr.Body.String())
			}
		})
	}
}
