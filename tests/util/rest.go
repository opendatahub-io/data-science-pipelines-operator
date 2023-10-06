package systemsTesttUtil

import (
	"bytes"
	. "github.com/onsi/gomega"
	"io"
	"mime/multipart"
	"os"
	"strings"
)

func FormFromFile(form map[string]string) (*bytes.Buffer, string) {
	body := new(bytes.Buffer)
	mp := multipart.NewWriter(body)
	defer mp.Close()
	for key, val := range form {
		if strings.HasPrefix(val, "@") {
			val = val[1:]
			file, err := os.Open(val)
			Expect(err).ToNot(HaveOccurred())
			defer file.Close()
			part, err := mp.CreateFormFile(key, val)
			Expect(err).ToNot(HaveOccurred())
			_, err = io.Copy(part, file)
			Expect(err).ToNot(HaveOccurred())
		} else {
			err := mp.WriteField(key, val)
			Expect(err).ToNot(HaveOccurred())
		}
	}
	return body, mp.FormDataContentType()
}
