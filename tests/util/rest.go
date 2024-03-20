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

package testUtil

import (
	"bytes"
	"io"
	"mime/multipart"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// FormFromFile creates a multipart form data from the provided form map where the values are paths to files.
// It returns a buffer containing the encoded form data and the content type of the form.
// Requires passing the testing.T object for error handling with Testify.
func FormFromFile(t *testing.T, form map[string]string) (*bytes.Buffer, string) {
	body := new(bytes.Buffer)
	mp := multipart.NewWriter(body)
	defer mp.Close()

	for key, val := range form {
		if strings.HasPrefix(val, "@") {
			val = val[1:]
			file, err := os.Open(val)
			require.NoError(t, err, "Opening file failed")
			defer file.Close()

			part, err := mp.CreateFormFile(key, val)
			require.NoError(t, err, "Creating form file failed")

			_, err = io.Copy(part, file)
			require.NoError(t, err, "Copying file content failed")
		} else {
			err := mp.WriteField(key, val)
			require.NoError(t, err, "Writing form field failed")
		}
	}

	return body, mp.FormDataContentType()
}
