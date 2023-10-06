package systemtests

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"io/ioutil"
	"net/http"
)

var _ = Describe("A successfully deployed DSPA", func() {
	It("Should successfully fetch experiments.", func() {
		response, err := http.Get(fmt.Sprintf("%s/apis/v1beta1/experiments", APIServerURL))
		Expect(err).ToNot(HaveOccurred())
		responseData, err := ioutil.ReadAll(response.Body)
		loggr.Info(string(responseData))

		Expect(err).ToNot(HaveOccurred())
		Expect(response.StatusCode).To(Equal(200))

	})
})
