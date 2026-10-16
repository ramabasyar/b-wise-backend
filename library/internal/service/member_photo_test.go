package service

import (
	"encoding/base64"
	"errors"
	"testing"
)

func TestDecodeMemberPhoto(t *testing.T) {
	jpeg := []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43}
	dataURL := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpeg)

	got, contentType, err := decodeMemberPhoto(dataURL)
	if err != nil {
		t.Fatalf("decodeMemberPhoto() error = %v", err)
	}
	if contentType != "image/jpeg" {
		t.Fatalf("content type = %q", contentType)
	}
	if string(got) != string(jpeg) {
		t.Fatalf("decoded bytes differ")
	}
}

func TestDecodeMemberPhotoRejectsInvalidInput(t *testing.T) {
	cases := []string{
		"",
		"data:image/png;base64,iVBORw0KGgo=",
		"data:image/jpeg;base64,not-base64",
		"data:image/jpeg;base64," + base64.StdEncoding.EncodeToString([]byte("not jpeg")),
	}
	for _, input := range cases {
		if _, _, err := decodeMemberPhoto(input); !errors.Is(err, ErrPhotoInvalid) {
			t.Fatalf("decodeMemberPhoto(%q) error = %v, want ErrPhotoInvalid", input, err)
		}
	}
}
