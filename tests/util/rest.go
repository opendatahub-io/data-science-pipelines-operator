/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
