//go:build test_systest

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

package systemtests

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	systemsTesttUtil "github.com/opendatahub-io/data-science-pipelines-operator/tests/util"
	"io/ioutil"
	"net/http"
)

var _ = Describe("A successfully deployed API Server", func() {
	It("Should successfully fetch pipelines.", func() {
		response, err := http.Get(fmt.Sprintf("%s/apis/v1beta1/pipelines", APIServerURL))
		Expect(err).ToNot(HaveOccurred())

		responseData, err := ioutil.ReadAll(response.Body)
		Expect(err).ToNot(HaveOccurred())
		Expect(response.StatusCode).To(Equal(200))
		loggr.Info(string(responseData))
	})

	It("Should successfully upload a pipeline.", func() {
		postUrl := fmt.Sprintf("%s/apis/v1beta1/pipelines/upload", APIServerURL)
		vals := map[string]string{
			"uploadfile": "@resources/test-pipeline-run.yaml",
		}
		body, contentType := systemsTesttUtil.FormFromFile(vals)

		response, err := http.Post(postUrl, contentType, body)
		Expect(err).ToNot(HaveOccurred())
		responseData, err := ioutil.ReadAll(response.Body)
		responseString := string(responseData)
		loggr.Info(responseString)
		Expect(err).ToNot(HaveOccurred())
		Expect(response.StatusCode).To(Equal(200))
	})
})
