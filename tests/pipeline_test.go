package systemtests

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
})
