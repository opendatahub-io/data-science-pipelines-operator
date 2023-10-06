//go:build test_systest

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
